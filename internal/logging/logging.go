// Package logging provides Outpost's structured logger. LogCall is the only
// way call events are logged, and its signature has no parameter through
// which tool-call arguments or results could flow — payloads are unloggable
// by construction, not by discipline (spec §10).
package logging

import (
	"io"
	"log/slog"
	"time"
)

// New returns a JSON structured logger writing to w.
func New(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// LogCall records one proxied MCP call. It accepts only metadata — upstream
// name, JSON-RPC method, tool name, duration, and outcome — never the
// call's arguments or result.
func LogCall(logger *slog.Logger, upstream, method, tool string, duration time.Duration, callErr error) {
	attrs := []any{
		slog.String("upstream", upstream),
		slog.String("method", method),
		slog.String("tool", tool),
		slog.Duration("duration", duration),
	}
	if callErr != nil {
		logger.Error("mcp call failed", append(attrs, slog.String("error", callErr.Error()))...)
		return
	}
	logger.Info("mcp call", attrs...)
}
