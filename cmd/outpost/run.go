package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/korneza/outpost/internal/config"
	"github.com/korneza/outpost/internal/logging"
	"github.com/korneza/outpost/internal/proxy"
	"github.com/korneza/outpost/internal/stdioupstream"
	"github.com/korneza/outpost/internal/store"
)

type runConfig struct {
	stateDB string
}

// runStdioWrapper implements `outpost run -- <server-cmd> [args...]`: it
// spawns command as a child process, wraps it with a
// stdioupstream.Caller, and runs every newline-delimited JSON-RPC
// request read from in through the exact same gate as HTTP mode
// (T1/breaker/pinning/cache/anomaly/reporter), writing each response as
// one line to out. Reads until in reaches EOF.
func runStdioWrapper(cfg runConfig, in io.Reader, out io.Writer, command string, args []string) error {
	st, err := store.Open(cfg.stateDB)
	if err != nil {
		return fmt.Errorf("open state db: %w", err)
	}
	defer st.Close()

	child, err := stdioupstream.New(command, args...)
	if err != nil {
		return fmt.Errorf("spawn child: %w", err)
	}
	defer child.Close()

	appCfg := &config.Config{Listen: "stdio", Upstreams: []config.Upstream{{Name: "stdio"}}}
	handler, err := proxy.NewSingle("stdio", child, appCfg, logging.New(io.Discard), st, nil)
	if err != nil {
		return fmt.Errorf("build handler: %w", err)
	}

	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		line := scanner.Bytes()
		resp := handler.Handle(context.Background(), line, "", "")
		if resp == nil {
			continue // notification — no response to write
		}
		body, err := json.Marshal(resp)
		if err != nil {
			continue
		}
		fmt.Fprintln(out, string(body))
	}
	return scanner.Err()
}
