package upstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	resp, err := client.Call(context.Background(), mcp.VersionCurrent, req)
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
	if _, err := client.Call(context.Background(), mcp.VersionNext, req); err != nil {
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
		if _, err := client.Call(context.Background(), mcp.VersionNext, toolsCallReq); err != nil {
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
		if _, err := client.Call(context.Background(), mcp.VersionCurrent, toolsCallReq); err != nil {
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
	if _, err := client.Call(context.Background(), mcp.VersionCurrent, req); err == nil {
		t.Fatal("expected an error calling an unreachable upstream, got nil")
	}
}
