package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/korneza/outpost/internal/config"
	"github.com/korneza/outpost/internal/logging"
	"github.com/korneza/outpost/internal/proxy"
	"github.com/korneza/outpost/internal/version"
)

// newServer builds the HTTP server that will run the proxy, from a loaded
// config. It does not start listening — callers decide how to run and shut
// it down.
func newServer(cfg *config.Config, logger *slog.Logger) (*http.Server, error) {
	handler, err := proxy.New(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("build proxy: %w", err)
	}
	return &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}, nil
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

	srv, err := newServer(cfg, logger)
	if err != nil {
		fmt.Fprintf(stderr, "outpost serve: %v\n", err)
		return 1
	}

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
