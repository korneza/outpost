// Command soaktest runs a real Outpost proxy against internal/chaos's
// chaotic upstream for a configurable duration — the actual Week-4
// "24-hour chaos soak" tool. Not run automatically by `go test`; invoke
// it directly for a real long-duration run:
//
//	go run ./cmd/soaktest -duration 24h
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/korneza/outpost/internal/chaos"
	"github.com/korneza/outpost/internal/config"
	"github.com/korneza/outpost/internal/logging"
	"github.com/korneza/outpost/internal/proxy"
	"github.com/korneza/outpost/internal/store"
)

func main() {
	duration := flag.Duration("duration", time.Hour, "how long to run the soak (e.g. 24h)")
	seed := flag.Int64("seed", time.Now().UnixNano(), "chaos RNG seed (fixed for a reproducible run)")
	statePath := flag.String("state-db", "soaktest.db", "path to the state SQLite file")
	flag.Parse()

	if err := run(*duration, *seed, *statePath); err != nil {
		fmt.Fprintf(os.Stderr, "soaktest: %v\n", err)
		os.Exit(1)
	}
}

func run(duration time.Duration, seed int64, statePath string) error {
	up := chaos.Upstream(seed)
	defer up.Close()

	cfg := &config.Config{
		Listen:    "127.0.0.1:0",
		StateDB:   statePath,
		Upstreams: []config.Upstream{{Name: "soak", URL: up.URL}},
	}
	st, err := store.Open(cfg.StateDB)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	handler, err := proxy.New(cfg, logging.New(os.Stdout), st, nil)
	if err != nil {
		return fmt.Errorf("build proxy: %w", err)
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()

	fmt.Fprintf(os.Stderr, "soaktest: running for %s against seed %d (proxy at %s)\n", duration, seed, srv.URL)

	client := &http.Client{Timeout: 5 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	calls, errs := 0, 0
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "soaktest: done — %d calls, %d transport errors (chaos-induced, expected)\n", calls, errs)
			return nil
		case <-ticker.C:
			fmt.Fprintf(os.Stderr, "soaktest: %d calls so far, %d transport errors\n", calls, errs)
		default:
			req, _ := http.NewRequest(http.MethodPost, srv.URL+"/soak", strings.NewReader(
				`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{}}}`,
			))
			resp, callErr := client.Do(req)
			calls++
			if callErr != nil {
				errs++
				continue
			}
			resp.Body.Close()
		}
	}
}
