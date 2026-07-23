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
	"github.com/korneza/outpost/internal/report"
	"github.com/korneza/outpost/internal/reporter"
	"github.com/korneza/outpost/internal/store"
	"github.com/korneza/outpost/internal/t1"
	"github.com/korneza/outpost/internal/tracing"
	"github.com/korneza/outpost/internal/upstream"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// caller is the minimal interface upstreamHandler needs from whatever it
// forwards calls to. *upstream.Client satisfies it today (HTTP);
// *stdioupstream.Caller (see internal/stdioupstream) satisfies it too, for
// `outpost run`'s stdio wrapper mode — the entire T1/breaker/pinning/
// cache/anomaly/reporter gate logic in ServeHTTP is transport-agnostic
// and shouldn't care which one it's talking to.
type caller interface {
	Call(ctx context.Context, version mcp.ProtocolVersion, req *mcp.Request, authHeader string) (*mcp.Response, error)
}

// New builds the proxy's HTTP handler from cfg: one route per configured
// upstream, at path "/{upstream.Name}". st backs each upstream's circuit
// breaker and pinning state for persistence. tp is nil-safe: a nil
// TracerProvider disables span emission entirely (same nil-safe pattern
// as the per-upstream cache).
func New(cfg *config.Config, logger *slog.Logger, st *store.Store, tp *sdktrace.TracerProvider) (http.Handler, error) {
	if len(cfg.Upstreams) == 0 {
		return nil, fmt.Errorf("proxy: at least one upstream is required")
	}
	var rep *reporter.Reporter
	if cfg.ControlPlaneURL != "" {
		rep = reporter.New(cfg.ControlPlaneURL, cfg.ControlPlaneAPIKey, 256)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
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
			name:     u.Name,
			client:   upstream.NewClient(u.URL),
			logger:   logger,
			t1:       t1.New(),
			breaker:  breaker.New(st, breaker.DefaultConfig()),
			pinning:  p,
			anomaly:  anomaly.New(),
			tools:    cfg.Tools,
			cache:    c,
			tp:       tp,
			reporter: rep,
		}
		mux.Handle("/"+u.Name, h)
	}
	return mux, nil
}

// NewSingle builds one upstreamHandler wired the same way New wires each
// per-upstream handler, for callers that don't want an HTTP mux — the
// stdio wrapper mode (cmd/outpost's "run" command) drives this directly
// from its own stdin/stdout read loop instead of over HTTP.
func NewSingle(name string, c caller, cfg *config.Config, logger *slog.Logger, st *store.Store, tp *sdktrace.TracerProvider) (*Handler, error) {
	p := pinning.New(st)
	if err := p.Hydrate(context.Background()); err != nil {
		logger.Error("pinning: failed to hydrate drift state from store", "upstream", name, "error", err)
	}
	var rep *reporter.Reporter
	if cfg.ControlPlaneURL != "" {
		rep = reporter.New(cfg.ControlPlaneURL, cfg.ControlPlaneAPIKey, 256)
	}
	return &Handler{h: &upstreamHandler{
		name:     name,
		client:   c,
		logger:   logger,
		t1:       t1.New(),
		breaker:  breaker.New(st, breaker.DefaultConfig()),
		pinning:  p,
		anomaly:  anomaly.New(),
		tools:    cfg.Tools,
		tp:       tp,
		reporter: rep,
	}}, nil
}

// Handler exposes upstreamHandler.handle to callers outside this package
// (the stdio wrapper mode) without exporting upstreamHandler itself.
type Handler struct{ h *upstreamHandler }

// Handle runs one JSON-RPC request through the full gate. See
// upstreamHandler.handle for what that means.
func (h *Handler) Handle(ctx context.Context, body []byte, authHeader, protocolVersionHeader string) *mcp.Response {
	return h.h.handle(ctx, body, authHeader, protocolVersionHeader)
}

type upstreamHandler struct {
	name     string
	client   caller
	logger   *slog.Logger
	t1       *t1.Validator
	breaker  *breaker.Breaker
	pinning  *pinning.Pinner
	anomaly  *anomaly.Detector
	tools    map[string]config.ToolOverride
	cache    *cache.Cache
	tp       *sdktrace.TracerProvider
	reporter *reporter.Reporter
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

	version := mcp.NegotiateVersion(r.Header.Get(mcp.ProtocolVersionHeader))
	w.Header().Set(mcp.ProtocolVersionHeader, string(version))

	resp := h.handle(r.Context(), body, r.Header.Get("Authorization"), r.Header.Get(mcp.ProtocolVersionHeader))
	writeResponse(w, resp)
}

// handle runs one JSON-RPC request through the full gate — T1, circuit
// breaker, pinning/drift, list-op cache, tracing, anomaly detection, and
// control-plane reporting — independent of transport. ServeHTTP (HTTP)
// and the stdio wrapper mode's read loop (internal/stdioupstream,
// cmd/outpost's "run" command) both call this directly so neither
// duplicates the gate logic.
func (h *upstreamHandler) handle(ctx context.Context, body []byte, authHeader, protocolVersionHeader string) *mcp.Response {
	var req mcp.Request
	if err := json.Unmarshal(body, &req); err != nil {
		return mcp.NewErrorResponse(nil, mcp.ParseError, "invalid JSON-RPC request")
	}

	version := mcp.NegotiateVersion(protocolVersionHeader)
	tool := mcp.ToolName(&req)

	if req.Method == mcp.MethodToolsCall {
		if !h.breaker.Allow(h.name, tool) {
			logging.LogCall(h.logger, h.name, req.Method, tool, 0, fmt.Errorf("circuit breaker open"))
			return mcp.NewErrorResponse(req.ID, mcp.CircuitOpen, "circuit breaker open for this tool")
		}
		if h.pinning.IsDrifted(h.name, tool) && h.tools[tool].Block {
			logging.LogCall(h.logger, h.name, req.Method, tool, 0, fmt.Errorf("blocked: tool definition drift detected"))
			return mcp.NewErrorResponse(req.ID, mcp.DriftBlocked, "tool definition changed since it was pinned; blocked per configuration")
		}
		if violation := h.t1.Check(tool, &req); violation != "" {
			logging.LogCall(h.logger, h.name, req.Method, tool, 0, fmt.Errorf("t1 rejected: %s", violation))
			return mcp.NewErrorResponse(req.ID, mcp.InvalidParams, violation)
		}
	}

	var cacheKey string
	if h.cache != nil {
		if key, ok := h.cache.Key(req.Method, h.name, req.Params, authHeader); ok {
			cacheKey = key
			if cached, hit := h.cache.Get(cacheKey); hit {
				logging.LogCall(h.logger, h.name, req.Method, tool, 0, nil)
				cachedResp := &mcp.Response{JSONRPC: "2.0", ID: req.ID, Result: cached}
				if req.Method == mcp.MethodToolsList {
					// Redaction is applied at serve time, not baked into
					// the cached bytes: IsDrifted state can change (or
					// first become known via Hydrate) after an entry was
					// cached, and a cache hit must reflect current
					// drift state exactly like a fresh fetch does.
					cachedResp = h.pinning.RedactDrifted(h.name, cachedResp)
				}
				return cachedResp
			}
		}
	}

	spanCtx := ctx
	var span trace.Span
	if h.tp != nil {
		spanCtx, span = tracing.StartCallSpan(spanCtx, h.tp, h.name, req.Method, tool)
	}

	start := time.Now()
	resp, callErr := h.client.Call(spanCtx, version, &req, authHeader)
	duration := time.Since(start)
	logging.LogCall(h.logger, h.name, req.Method, tool, duration, callErr)

	if span != nil {
		tracing.EndCallSpan(span, float64(duration.Milliseconds()), callErr == nil)
	}

	if cacheKey != "" && callErr == nil && resp != nil && resp.Error == nil {
		h.cache.Set(cacheKey, resp.Result)
	}

	if req.Method == mcp.MethodToolsCall {
		success := callErr == nil && (resp == nil || resp.Error == nil)
		if err := h.breaker.RecordResult(ctx, h.name, tool, success); err != nil {
			h.logger.Error("breaker: failed to persist state transition", "error", err)
		}
		for _, a := range h.anomaly.Observe(h.name, tool, float64(duration.Milliseconds()), !success) {
			h.logger.Warn("statistical anomaly detected", "upstream", a.Upstream, "tool", a.ToolName, "metric", a.Metric, "value", a.Value, "mean", a.Mean, "stddev", a.StdDev)
		}
	}

	if callErr == nil && req.Method == mcp.MethodToolsList {
		h.t1.LearnFromToolsList(resp)
		if alerts, err := h.pinning.LearnFromToolsList(ctx, h.name, resp); err != nil {
			h.logger.Error("pinning: failed to process tools/list", "error", err)
		} else {
			for _, a := range alerts {
				h.logger.Error("tool definition drift detected", "upstream", a.Upstream, "tool", a.ToolName, "old_hash", a.OldHash, "new_hash", a.NewHash)
				if h.reporter != nil {
					h.reporter.ReportDrift(report.DriftEvent{Upstream: a.Upstream, ToolName: a.ToolName, OldHash: a.OldHash, NewHash: a.NewHash, ToolDef: a.ToolDef, DetectedAt: time.Now().UTC()})
				}
			}
		}
	}

	if callErr != nil {
		return mcp.NewErrorResponse(req.ID, mcp.InternalError, "upstream call failed")
	}
	if req.Method == mcp.MethodToolsList {
		resp = h.pinning.RedactDrifted(h.name, resp)
	}
	return resp
}

func writeResponse(w http.ResponseWriter, resp *mcp.Response) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
