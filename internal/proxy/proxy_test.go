package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/korneza/outpost/internal/config"
	"github.com/korneza/outpost/internal/logging"
	"github.com/korneza/outpost/internal/mcp"
	"github.com/korneza/outpost/internal/store"
)

func fakeUpstream(t *testing.T, respond func(mcp.Request) mcp.Response) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mcp.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("fake upstream: decode: %v", err)
		}
		resp := respond(req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func newTestProxy(t *testing.T, upstreamURL string) (http.Handler, *bytes.Buffer) {
	t.Helper()
	cfg := &config.Config{
		Listen:    "127.0.0.1:0",
		Upstreams: []config.Upstream{{Name: "files", URL: upstreamURL}},
	}
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	var logBuf bytes.Buffer
	handler, err := New(cfg, logging.New(&logBuf), st)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return handler, &logBuf
}

func TestProxyForwardsToolsCallToUpstream(t *testing.T) {
	up := fakeUpstream(t, func(req mcp.Request) mcp.Response {
		return mcp.Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"content":"hello"}`)}
	})
	defer up.Close()

	handler, _ := newTestProxy(t, up.URL)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"files.read","arguments":{}}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/files", strings.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var resp mcp.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if string(resp.Result) != `{"content":"hello"}` {
		t.Fatalf("Result = %s, want {\"content\":\"hello\"}", resp.Result)
	}
}

func TestProxyRejectsMalformedJSON(t *testing.T) {
	up := fakeUpstream(t, func(req mcp.Request) mcp.Response {
		t.Fatal("upstream should not be called for malformed input")
		return mcp.Response{}
	})
	defer up.Close()

	handler, _ := newTestProxy(t, up.URL)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/files", strings.NewReader(`not json`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (JSON-RPC errors are transport-200)", rec.Code)
	}
	var resp mcp.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != mcp.ParseError {
		t.Fatalf("Error = %+v, want code %d", resp.Error, mcp.ParseError)
	}
}

func TestProxyRejectsGET(t *testing.T) {
	up := fakeUpstream(t, func(req mcp.Request) mcp.Response { return mcp.Response{} })
	defer up.Close()

	handler, _ := newTestProxy(t, up.URL)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/files", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestProxyUnknownUpstreamIs404(t *testing.T) {
	up := fakeUpstream(t, func(req mcp.Request) mcp.Response { return mcp.Response{} })
	defer up.Close()

	handler, _ := newTestProxy(t, up.URL)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/does-not-exist", strings.NewReader(`{}`))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestProxyLogsToolNameWithNoPayloadFields(t *testing.T) {
	up := fakeUpstream(t, func(req mcp.Request) mcp.Response {
		return mcp.Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"content":"secret-looking-value"}`)}
	})
	defer up.Close()

	handler, logBuf := newTestProxy(t, up.URL)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"files.read","arguments":{"path":"/etc/shadow"}}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/files", strings.NewReader(body))
	handler.ServeHTTP(rec, req)

	logged := logBuf.String()
	if strings.Contains(logged, "/etc/shadow") {
		t.Fatalf("log line leaked call arguments: %s", logged)
	}
	if strings.Contains(logged, "secret-looking-value") {
		t.Fatalf("log line leaked call result: %s", logged)
	}
	if !strings.Contains(logged, "files.read") {
		t.Fatalf("expected log line to include the tool name files.read: %s", logged)
	}
}

func TestProxySetsNegotiatedVersionOnResponse(t *testing.T) {
	up := fakeUpstream(t, func(req mcp.Request) mcp.Response {
		return mcp.Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{}`)}
	})
	defer up.Close()

	handler, _ := newTestProxy(t, up.URL)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/files", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set(mcp.ProtocolVersionHeader, string(mcp.VersionNext))
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get(mcp.ProtocolVersionHeader); got != string(mcp.VersionNext) {
		t.Fatalf("%s response header = %q, want %q", mcp.ProtocolVersionHeader, got, mcp.VersionNext)
	}
}

func TestProxyRejectsInvalidToolCallWithoutReachingUpstream(t *testing.T) {
	var toolsCallCount int
	up := fakeUpstream(t, func(req mcp.Request) mcp.Response {
		if req.Method == mcp.MethodToolsList {
			return mcp.Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(
				`{"tools":[{"name":"files.read","inputSchema":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}}]}`,
			)}
		}
		toolsCallCount++
		return mcp.Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"content":"should not be reached"}`)}
	})
	defer up.Close()

	handler, _ := newTestProxy(t, up.URL)

	// First, tools/list — the proxy learns files.read's schema.
	listRec := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodPost, "/files", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d, want 200", listRec.Code)
	}

	// Then, an invalid tools/call — missing the required "path" argument.
	callRec := httptest.NewRecorder()
	callBody := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"files.read","arguments":{}}}`
	callReq := httptest.NewRequest(http.MethodPost, "/files", strings.NewReader(callBody))
	handler.ServeHTTP(callRec, callReq)

	if callRec.Code != http.StatusOK {
		t.Fatalf("tools/call status = %d, want 200 (JSON-RPC errors are transport-200)", callRec.Code)
	}
	var resp mcp.Response
	if err := json.Unmarshal(callRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != mcp.InvalidParams {
		t.Fatalf("Error = %+v, want code %d (InvalidParams)", resp.Error, mcp.InvalidParams)
	}
	if toolsCallCount != 0 {
		t.Fatalf("upstream saw %d tools/call requests, want 0 — T1 must reject before forwarding", toolsCallCount)
	}
}

func TestProxyForwardsValidToolCallAfterLearningSchema(t *testing.T) {
	up := fakeUpstream(t, func(req mcp.Request) mcp.Response {
		if req.Method == mcp.MethodToolsList {
			return mcp.Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(
				`{"tools":[{"name":"files.read","inputSchema":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}}]}`,
			)}
		}
		return mcp.Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"content":"real content"}`)}
	})
	defer up.Close()

	handler, _ := newTestProxy(t, up.URL)

	listRec := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodPost, "/files", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	handler.ServeHTTP(listRec, listReq)

	callRec := httptest.NewRecorder()
	callBody := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"files.read","arguments":{"path":"/tmp/x"}}}`
	callReq := httptest.NewRequest(http.MethodPost, "/files", strings.NewReader(callBody))
	handler.ServeHTTP(callRec, callReq)

	var resp mcp.Response
	if err := json.Unmarshal(callRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error for a valid call: %+v", resp.Error)
	}
	if string(resp.Result) != `{"content":"real content"}` {
		t.Fatalf("Result = %s, want {\"content\":\"real content\"}", resp.Result)
	}
}

func TestProxyStillForwardsToolCallForUnlearnedTool(t *testing.T) {
	// Regression guard: T1 must stay fail-open for tools it has never seen
	// via tools/list — this is what keeps existing behavior (and existing
	// tests, like TestProxyForwardsToolsCallToUpstream) unbroken.
	up := fakeUpstream(t, func(req mcp.Request) mcp.Response {
		return mcp.Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"content":"ok"}`)}
	})
	defer up.Close()

	handler, _ := newTestProxy(t, up.URL)
	rec := httptest.NewRecorder()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"never.listed","arguments":{"anything":"goes"}}}`
	req := httptest.NewRequest(http.MethodPost, "/files", strings.NewReader(body))
	handler.ServeHTTP(rec, req)

	var resp mcp.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error for an unlearned tool: %+v — T1 must fail open", resp.Error)
	}
}

func TestProxyOpensCircuitAfterConsecutiveFailures(t *testing.T) {
	up := fakeUpstream(t, func(req mcp.Request) mcp.Response {
		if req.Method == mcp.MethodToolsList {
			return mcp.Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"tools":[]}`)}
		}
		return mcp.Response{JSONRPC: "2.0", ID: req.ID, Error: &mcp.Error{Code: mcp.InternalError, Message: "upstream is broken"}}
	})
	defer up.Close()

	handler, _ := newTestProxy(t, up.URL)

	for i := 0; i < 5; i++ { // default threshold
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/files", strings.NewReader(
			`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"broken.tool","arguments":{}}}`,
		))
		handler.ServeHTTP(rec, req)
	}

	// One more call: the breaker must now be open, rejecting before the
	// (still-broken, but that's not the point) upstream is even reached.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/files", strings.NewReader(
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"broken.tool","arguments":{}}}`,
	))
	handler.ServeHTTP(rec, req)
	var resp mcp.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != mcp.CircuitOpen {
		t.Fatalf("Error = %+v, want code %d (CircuitOpen)", resp.Error, mcp.CircuitOpen)
	}
}
