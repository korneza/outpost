package upstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/korneza/outpost/internal/mcp"
)

func TestCallSendsAndDecodesJSONRPC(t *testing.T) {
	var gotReq mcp.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("server: decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(gotReq.ID) + `,"result":{"ok":true}}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: mcp.MethodToolsList}
	resp, err := client.Call(context.Background(), mcp.VersionCurrent, req, "")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error in response: %+v", resp.Error)
	}
	if string(resp.Result) != `{"ok":true}` {
		t.Fatalf("Result = %s, want {\"ok\":true}", resp.Result)
	}
	if gotReq.Method != mcp.MethodToolsList {
		t.Fatalf("server saw method %q, want %q", gotReq.Method, mcp.MethodToolsList)
	}
}

func TestCallSetsProtocolVersionHeader(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get(mcp.ProtocolVersionHeader)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: mcp.MethodToolsList}
	if _, err := client.Call(context.Background(), mcp.VersionNext, req, ""); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if gotHeader != string(mcp.VersionNext) {
		t.Fatalf("%s header = %q, want %q", mcp.ProtocolVersionHeader, gotHeader, mcp.VersionNext)
	}
}

func TestCallSetsRoutingHeadersOnlyForVersionNext(t *testing.T) {
	toolsCallReq := &mcp.Request{
		JSONRPC: "2.0", ID: json.RawMessage("1"), Method: mcp.MethodToolsCall,
		Params: json.RawMessage(`{"name":"files.read","arguments":{}}`),
	}

	t.Run("VersionNext sets Mcp-Method and Mcp-Name", func(t *testing.T) {
		var gotMethod, gotName string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Header.Get("Mcp-Method")
			gotName = r.Header.Get("Mcp-Name")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
		}))
		defer srv.Close()

		client := NewClient(srv.URL)
		if _, err := client.Call(context.Background(), mcp.VersionNext, toolsCallReq, ""); err != nil {
			t.Fatalf("Call: %v", err)
		}
		if gotMethod != mcp.MethodToolsCall {
			t.Errorf("Mcp-Method = %q, want %q", gotMethod, mcp.MethodToolsCall)
		}
		if gotName != "files.read" {
			t.Errorf("Mcp-Name = %q, want %q", gotName, "files.read")
		}
	})

	t.Run("VersionCurrent omits routing headers", func(t *testing.T) {
		var sawMethod, sawName bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, sawMethod = r.Header["Mcp-Method"]
			_, sawName = r.Header["Mcp-Name"]
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
		}))
		defer srv.Close()

		client := NewClient(srv.URL)
		if _, err := client.Call(context.Background(), mcp.VersionCurrent, toolsCallReq, ""); err != nil {
			t.Fatalf("Call: %v", err)
		}
		if sawMethod || sawName {
			t.Errorf("VersionCurrent must not set routing headers: Mcp-Method present=%v Mcp-Name present=%v", sawMethod, sawName)
		}
	})
}

func TestCallReturnsErrorOnUnreachableUpstream(t *testing.T) {
	client := NewClient("http://127.0.0.1:1") // reserved port, nothing listens here
	req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: mcp.MethodToolsList}
	if _, err := client.Call(context.Background(), mcp.VersionCurrent, req, ""); err == nil {
		t.Fatal("expected an error calling an unreachable upstream, got nil")
	}
}

// TestCallRejectsOversizedResponseBody guards against Claude Security
// findings F5/F16: resp.Body was read in full with io.ReadAll and no
// size cap, so a hostile or compromised upstream could force Outpost to
// buffer an arbitrarily large response before any check ran, exhausting
// memory. maxResponseBytes is a package var here specifically so the
// test doesn't need to actually transfer tens of megabytes to prove the
// cap holds.
func TestCallRejectsOversizedResponseBody(t *testing.T) {
	orig := maxResponseBytes
	maxResponseBytes = 100
	t.Cleanup(func() { maxResponseBytes = orig })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"` + strings.Repeat("a", 1000) + `"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: mcp.MethodToolsList}
	if _, err := client.Call(context.Background(), mcp.VersionCurrent, req, ""); err == nil {
		t.Fatal("expected an error for a response body over the size cap, got nil")
	}
}

// TestCallAcceptsResponseAtCap confirms the cap doesn't clip a
// legitimate response sitting right at the limit.
func TestCallAcceptsResponseAtCap(t *testing.T) {
	orig := maxResponseBytes
	maxResponseBytes = 1000
	t.Cleanup(func() { maxResponseBytes = orig })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := `{"jsonrpc":"2.0","id":1,"result":"`
		suffix := `"}`
		padding := int(maxResponseBytes) - len(prefix) - len(suffix)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(prefix + strings.Repeat("a", padding) + suffix))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: mcp.MethodToolsList}
	if _, err := client.Call(context.Background(), mcp.VersionCurrent, req, ""); err != nil {
		t.Fatalf("Call: %v, want a response exactly at the cap to still succeed", err)
	}
}

func TestCallForwardsAuthorizationHeaderWhenPresent(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: mcp.MethodToolsList}
	if _, err := client.Call(context.Background(), mcp.VersionCurrent, req, "Bearer opaque-token-value"); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if gotAuth != "Bearer opaque-token-value" {
		t.Fatalf("Authorization = %q, want %q — Outpost forwards opaque bearer tokens, never brokers them", gotAuth, "Bearer opaque-token-value")
	}
}

func TestCallOmitsAuthorizationHeaderWhenAbsent(t *testing.T) {
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawAuth = r.Header["Authorization"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: mcp.MethodToolsList}
	if _, err := client.Call(context.Background(), mcp.VersionCurrent, req, ""); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if sawAuth {
		t.Fatal("Authorization header present on the outgoing request when the client sent none")
	}
}
