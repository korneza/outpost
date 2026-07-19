// Package proxy implements Outpost's reverse-proxy HTTP handler: it accepts
// MCP client requests, forwards them to the configured upstream, and
// returns the upstream's response — logging only call metadata along the
// way (see internal/logging).
package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/korneza/outpost/internal/breaker"
	"github.com/korneza/outpost/internal/config"
	"github.com/korneza/outpost/internal/logging"
	"github.com/korneza/outpost/internal/mcp"
	"github.com/korneza/outpost/internal/store"
	"github.com/korneza/outpost/internal/t1"
	"github.com/korneza/outpost/internal/upstream"
)

// New builds the proxy's HTTP handler from cfg: one route per configured
// upstream, at path "/{upstream.Name}". st backs each upstream's circuit
// breaker for state-transition persistence.
func New(cfg *config.Config, logger *slog.Logger, st *store.Store) (http.Handler, error) {
	if len(cfg.Upstreams) == 0 {
		return nil, fmt.Errorf("proxy: at least one upstream is required")
	}
	mux := http.NewServeMux()
	for _, u := range cfg.Upstreams {
		h := &upstreamHandler{
			name:    u.Name,
			client:  upstream.NewClient(u.URL),
			logger:  logger,
			t1:      t1.New(),
			breaker: breaker.New(st, breaker.DefaultConfig()),
		}
		mux.Handle("/"+u.Name, h)
	}
	return mux, nil
}

type upstreamHandler struct {
	name    string
	client  *upstream.Client
	logger  *slog.Logger
	t1      *t1.Validator
	breaker *breaker.Breaker
}

func (h *upstreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	var req mcp.Request
	if err := json.Unmarshal(body, &req); err != nil {
		writeResponse(w, mcp.NewErrorResponse(nil, mcp.ParseError, "invalid JSON-RPC request"))
		return
	}

	version := mcp.NegotiateVersion(r.Header.Get(mcp.ProtocolVersionHeader))
	tool := mcp.ToolName(&req)
	w.Header().Set(mcp.ProtocolVersionHeader, string(version))

	if req.Method == mcp.MethodToolsCall {
		if !h.breaker.Allow(h.name, tool) {
			logging.LogCall(h.logger, h.name, req.Method, tool, 0, fmt.Errorf("circuit breaker open"))
			writeResponse(w, mcp.NewErrorResponse(req.ID, mcp.CircuitOpen, "circuit breaker open for this tool"))
			return
		}
		if violation := h.t1.Check(tool, &req); violation != "" {
			logging.LogCall(h.logger, h.name, req.Method, tool, 0, fmt.Errorf("t1 rejected: %s", violation))
			writeResponse(w, mcp.NewErrorResponse(req.ID, mcp.InvalidParams, violation))
			return
		}
	}

	start := time.Now()
	resp, callErr := h.client.Call(r.Context(), version, &req)
	duration := time.Since(start)
	logging.LogCall(h.logger, h.name, req.Method, tool, duration, callErr)

	if req.Method == mcp.MethodToolsCall {
		success := callErr == nil && (resp == nil || resp.Error == nil)
		if err := h.breaker.RecordResult(r.Context(), h.name, tool, success); err != nil {
			h.logger.Error("breaker: failed to persist state transition", "error", err)
		}
	}

	if callErr == nil && req.Method == mcp.MethodToolsList {
		h.t1.LearnFromToolsList(resp)
	}

	if callErr != nil {
		writeResponse(w, mcp.NewErrorResponse(req.ID, mcp.InternalError, "upstream call failed"))
		return
	}
	writeResponse(w, resp)
}

func writeResponse(w http.ResponseWriter, resp *mcp.Response) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
