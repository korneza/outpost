package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/korneza/outpost/internal/config"
	"github.com/korneza/outpost/internal/logging"
	"github.com/korneza/outpost/internal/mcp"
)

func TestNewServerUsesConfiguredListenAddress(t *testing.T) {
	cfg := &config.Config{
		Listen:    "127.0.0.1:8123",
		Upstreams: []config.Upstream{{Name: "files", URL: "http://127.0.0.1:9999/mcp"}},
	}
	srv, err := newServer(cfg, logging.New(io.Discard))
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	if srv.Addr != "127.0.0.1:8123" {
		t.Fatalf("Addr = %q, want %q", srv.Addr, "127.0.0.1:8123")
	}
	if srv.Handler == nil {
		t.Fatal("expected a non-nil Handler")
	}
}

func TestNewServerRejectsConfigWithNoUpstreams(t *testing.T) {
	cfg := &config.Config{Listen: "127.0.0.1:8123"}
	if _, err := newServer(cfg, logging.New(io.Discard)); err == nil {
		t.Fatal("expected an error building a server with no upstreams")
	}
}

func TestRunServeFailsCleanlyOnMissingConfig(t *testing.T) {
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()

	code := runServe(filepath.Join(t.TempDir(), "does-not-exist.yaml"), stdout, stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

func TestServeEndToEndOverRealListener(t *testing.T) {
	fakeUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":"pong from reference upstream"}}`))
	}))
	defer fakeUpstream.Close()

	configPath := filepath.Join(t.TempDir(), "outpost.yaml")
	configYAML := "listen: \"127.0.0.1:0\"\nupstreams:\n  - name: files\n    url: \"" + fakeUpstream.URL + "\"\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	srv, err := newServer(cfg, logging.New(io.Discard))
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"files.read","arguments":{}}}`
	resp, err := http.Post("http://"+ln.Addr().String()+"/files", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	var decoded mcp.Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, data)
	}
	if string(decoded.Result) != `{"content":"pong from reference upstream"}` {
		t.Fatalf("Result = %s, want {\"content\":\"pong from reference upstream\"}", decoded.Result)
	}
}

func TestServeEndToEndRejectsInvalidToolCall(t *testing.T) {
	var toolsCallCount int
	fakeUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req mcp.Request
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if req.Method == mcp.MethodToolsList {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"files.read","inputSchema":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}}]}}`))
			return
		}
		toolsCallCount++
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"content":"should not be reached"}}`))
	}))
	defer fakeUpstream.Close()

	configPath := filepath.Join(t.TempDir(), "outpost.yaml")
	configYAML := "listen: \"127.0.0.1:0\"\nupstreams:\n  - name: files\n    url: \"" + fakeUpstream.URL + "\"\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	srv, err := newServer(cfg, logging.New(io.Discard))
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	base := "http://" + ln.Addr().String() + "/files"

	listResp, err := http.Post(base, "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatalf("tools/list POST: %v", err)
	}
	listResp.Body.Close()

	callResp, err := http.Post(base, "application/json", strings.NewReader(
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"files.read","arguments":{}}}`,
	))
	if err != nil {
		t.Fatalf("tools/call POST: %v", err)
	}
	defer callResp.Body.Close()
	data, err := io.ReadAll(callResp.Body)
	if err != nil {
		t.Fatal(err)
	}

	var decoded mcp.Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, data)
	}
	if decoded.Error == nil || decoded.Error.Code != mcp.InvalidParams {
		t.Fatalf("Error = %+v, want code %d (InvalidParams)", decoded.Error, mcp.InvalidParams)
	}
	if toolsCallCount != 0 {
		t.Fatalf("upstream saw %d tools/call requests over the real listener, want 0", toolsCallCount)
	}
}
