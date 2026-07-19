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
	"sync"
	"testing"

	"github.com/korneza/outpost/internal/config"
	"github.com/korneza/outpost/internal/logging"
	"github.com/korneza/outpost/internal/mcp"
)

// TestServeSupportsBothProtocolVersionsEndToEnd is the Week-1 exit-criteria
// demo from the 30-day plan: outpost serve proxies a reference MCP server
// correctly on both 2025-11-25 and 2026-07-28, through the real compiled
// server (not isolated unit tests of the negotiation logic alone).
func TestServeSupportsBothProtocolVersionsEndToEnd(t *testing.T) {
	var mu sync.Mutex
	var lastMcpMethodHeader, lastMcpNameHeader string

	fakeUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastMcpMethodHeader = r.Header.Get("Mcp-Method")
		lastMcpNameHeader = r.Header.Get("Mcp-Name")
		mu.Unlock()

		var req mcp.Request
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if req.Method == mcp.MethodToolsList {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"files.read","inputSchema":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"content":"reached upstream"}}`))
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

	for _, tc := range []struct {
		name              string
		protocolHeader    string // "" means omit the header entirely
		wantMcpHeadersSet bool
	}{
		{name: "VersionCurrent (no header, the compatibility default)", protocolHeader: "", wantMcpHeadersSet: false},
		{name: "VersionNext (2026-07-28)", protocolHeader: string(mcp.VersionNext), wantMcpHeadersSet: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			post := func(body string) *mcp.Response {
				t.Helper()
				req, err := http.NewRequest(http.MethodPost, base, strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				if tc.protocolHeader != "" {
					req.Header.Set(mcp.ProtocolVersionHeader, tc.protocolHeader)
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("POST: %v", err)
				}
				defer resp.Body.Close()

				wantVersion := mcp.VersionCurrent
				if tc.protocolHeader != "" {
					wantVersion = mcp.VersionNext
				}
				if got := resp.Header.Get(mcp.ProtocolVersionHeader); got != string(wantVersion) {
					t.Errorf("response %s = %q, want %q", mcp.ProtocolVersionHeader, got, wantVersion)
				}

				data, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatal(err)
				}
				var decoded mcp.Response
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatalf("decode response: %v; body=%s", err, data)
				}
				return &decoded
			}

			// Learn the schema.
			post(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)

			// Invalid call: missing required "path".
			invalid := post(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"files.read","arguments":{}}}`)
			if invalid.Error == nil || invalid.Error.Code != mcp.InvalidParams {
				t.Fatalf("invalid call: Error = %+v, want code %d — T1 must reject regardless of protocol version", invalid.Error, mcp.InvalidParams)
			}

			// Valid call: reaches upstream.
			valid := post(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"files.read","arguments":{"path":"/tmp/x"}}}`)
			if valid.Error != nil {
				t.Fatalf("valid call: unexpected error %+v", valid.Error)
			}
			if string(valid.Result) != `{"content":"reached upstream"}` {
				t.Fatalf("valid call: Result = %s, want reached-upstream response", valid.Result)
			}

			mu.Lock()
			gotHeadersSet := lastMcpMethodHeader != "" || lastMcpNameHeader != ""
			mu.Unlock()
			if gotHeadersSet != tc.wantMcpHeadersSet {
				t.Errorf("Mcp-Method/Mcp-Name headers set on the forwarded request = %v, want %v", gotHeadersSet, tc.wantMcpHeadersSet)
			}
		})
	}
}
