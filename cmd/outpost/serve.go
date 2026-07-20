package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/korneza/outpost/internal/config"
	"github.com/korneza/outpost/internal/logging"
	"github.com/korneza/outpost/internal/proxy"
	"github.com/korneza/outpost/internal/store"
	"github.com/korneza/outpost/internal/tracing"
	"github.com/korneza/outpost/internal/version"
)

// newServer builds the HTTP server that will run the proxy, from a loaded
// config. It does not start listening — callers decide how to run and shut
// it down. The returned *store.Store is the caller's to close; it opens
// cfg.StateDB but does not own its lifecycle beyond that. traceWriter
// receives exported OTel spans (os.Stdout in production; tests pass
// io.Discard to keep test output clean). The tracer provider's shutdown
// is registered on the server itself via RegisterOnShutdown, so callers
// don't need to manage its lifecycle separately — a graceful
// srv.Shutdown flushes it automatically.
func newServer(cfg *config.Config, logger *slog.Logger, traceWriter io.Writer) (*http.Server, *store.Store, error) {
	st, err := store.Open(cfg.StateDB)
	if err != nil {
		return nil, nil, fmt.Errorf("open state db: %w", err)
	}
	tp, err := tracing.NewProvider(traceWriter)
	if err != nil {
		st.Close()
		return nil, nil, fmt.Errorf("build tracer provider: %w", err)
	}
	handler, err := proxy.New(cfg, logger, st, tp)
	if err != nil {
		st.Close()
		if shutdownErr := tp.Shutdown(context.Background()); shutdownErr != nil {
			logger.Error("tracing: failed to shut down tracer provider", "error", shutdownErr)
		}
		return nil, nil, fmt.Errorf("build proxy: %w", err)
	}
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	srv.RegisterOnShutdown(func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			logger.Error("tracing: failed to shut down tracer provider", "error", err)
		}
	})
	return srv, st, nil
}

// runServe loads configPath, starts the proxy server, and blocks until an
// interrupt or terminate signal triggers a graceful shutdown. It returns a
// process exit code.
func runServe(configPath string, stdout, stderr *os.File) int {
	logger := logging.New(stdout)

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "outpost serve: %v\n", err)
		return 1
	}

	srv, st, err := newServer(cfg, logger, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "outpost serve: %v\n", err)
		return 1
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(stdout, "%s\nlistening on %s\n", version.String(), cfg.Listen)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(stderr, "outpost serve: %v\n", err)
			return 1
		}
	case <-ctx.Done():
		fmt.Fprintln(stdout, "shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(stderr, "outpost serve: shutdown: %v\n", err)
			return 1
		}
	}
	return 0
}
