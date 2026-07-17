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
	var logBuf bytes.Buffer
	handler, err := New(cfg, logging.New(&logBuf))
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
