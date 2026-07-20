// Package proxy implements Outpost's reverse-proxy HTTP handler: it accepts
// MCP client requests, forwards them to the configured upstream, and
// returns the upstream's response — logging only call metadata along the
// way (see internal/logging).
package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/korneza/outpost/internal/anomaly"
	"github.com/korneza/outpost/internal/breaker"
	"github.com/korneza/outpost/internal/cache"
	"github.com/korneza/outpost/internal/config"
	"github.com/korneza/outpost/internal/logging"
	"github.com/korneza/outpost/internal/mcp"
	"github.com/korneza/outpost/internal/pinning"
	"github.com/korneza/outpost/internal/store"
	"github.com/korneza/outpost/internal/t1"
	"github.com/korneza/outpost/internal/upstream"
)

// New builds the proxy's HTTP handler from cfg: one route per configured
// upstream, at path "/{upstream.Name}". st backs each upstream's circuit
// breaker and pinning state for persistence.
func New(cfg *config.Config, logger *slog.Logger, st *store.Store) (http.Handler, error) {
	if len(cfg.Upstreams) == 0 {
		return nil, fmt.Errorf("proxy: at least one upstream is required")
	}
	mux := http.NewServeMux()
	for _, u := range cfg.Upstreams {
		p := pinning.New(st)
		// Restore drift-block state from persisted history — otherwise a
		// restart silently un-blocks a previously drifted tool. A
		// hydration failure is fail-open (logged, not fatal): it leaves
		// block state as if no drift had ever been seen, same as before
		// this method existed, rather than refusing to start the proxy.
		if err := p.Hydrate(context.Background()); err != nil {
			logger.Error("pinning: failed to hydrate drift state from store", "upstream", u.Name, "error", err)
		}
		var c *cache.Cache
		if u.CacheTTLSeconds > 0 {
			c = cache.New(time.Duration(u.CacheTTLSeconds) * time.Second)
		}
		h := &upstreamHandler{
			name:    u.Name,
			client:  upstream.NewClient(u.URL),
			logger:  logger,
			t1:      t1.New(),
			breaker: breaker.New(st, breaker.DefaultConfig()),
			pinning: p,
			anomaly: anomaly.New(),
			tools:   cfg.Tools,
			cache:   c,
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
	pinning *pinning.Pinner
	anomaly *anomaly.Detector
	tools   map[string]config.ToolOverride
	cache   *cache.Cache
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
		if h.pinning.IsDrifted(h.name, tool) && h.tools[tool].Block {
			logging.LogCall(h.logger, h.name, req.Method, tool, 0, fmt.Errorf("blocked: tool definition drift detected"))
			writeResponse(w, mcp.NewErrorResponse(req.ID, mcp.DriftBlocked, "tool definition changed since it was pinned; blocked per configuration"))
			return
		}
		if violation := h.t1.Check(tool, &req); violation != "" {
			logging.LogCall(h.logger, h.name, req.Method, tool, 0, fmt.Errorf("t1 rejected: %s", violation))
			writeResponse(w, mcp.NewErrorResponse(req.ID, mcp.InvalidParams, violation))
			return
		}
	}

	var cacheKey string
	if h.cache != nil {
		if key, ok := h.cache.Key(req.Method, h.name, req.Params); ok {
			cacheKey = key
			if cached, hit := h.cache.Get(cacheKey); hit {
				logging.LogCall(h.logger, h.name, req.Method, tool, 0, nil)
				writeResponse(w, &mcp.Response{JSONRPC: "2.0", ID: req.ID, Result: cached})
				return
			}
		}
	}

	start := time.Now()
	resp, callErr := h.client.Call(r.Context(), version, &req, r.Header.Get("Authorization"))
	duration := time.Since(start)
	logging.LogCall(h.logger, h.name, req.Method, tool, duration, callErr)

	if cacheKey != "" && callErr == nil && resp != nil && resp.Error == nil {
		h.cache.Set(cacheKey, resp.Result)
	}

	if req.Method == mcp.MethodToolsCall {
		success := callErr == nil && (resp == nil || resp.Error == nil)
		if err := h.breaker.RecordResult(r.Context(), h.name, tool, success); err != nil {
			h.logger.Error("breaker: failed to persist state transition", "error", err)
		}
		for _, a := range h.anomaly.Observe(h.name, tool, float64(duration.Milliseconds()), !success) {
			h.logger.Warn("statistical anomaly detected", "upstream", a.Upstream, "tool", a.ToolName, "metric", a.Metric, "value", a.Value, "mean", a.Mean, "stddev", a.StdDev)
		}
	}

	if callErr == nil && req.Method == mcp.MethodToolsList {
		h.t1.LearnFromToolsList(resp)
		if alerts, err := h.pinning.LearnFromToolsList(r.Context(), h.name, resp); err != nil {
			h.logger.Error("pinning: failed to process tools/list", "error", err)
		} else {
			for _, a := range alerts {
				h.logger.Error("tool definition drift detected", "upstream", a.Upstream, "tool", a.ToolName, "old_hash", a.OldHash, "new_hash", a.NewHash)
			}
		}
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
