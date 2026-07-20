package mcp

import (
	"encoding/json"
	"testing"
)

// FuzzUnmarshalRequest feeds arbitrary bytes at the exact boundary where
// attacker-controlled traffic first touches Outpost: decoding an inbound
// HTTP request body as a JSON-RPC Request. It must never panic — a crash
// here is an unauthenticated remote DoS against a proxy this product's
// entire purpose is to make more reliable than what it fronts.
func FuzzUnmarshalRequest(f *testing.F) {
	seeds := []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"x","arguments":{}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`not json`,
		`{}`,
		`null`,
		`[]`,
		`{"id":null}`,
		`{"id":{"nested":"object"}}`,
		`{"params":"not an object"}`,
		`{"id":1e400}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(_ *testing.T, data []byte) {
		var req Request
		_ = json.Unmarshal(data, &req) // error is fine; panic is not

		// A successfully-decoded request must round-trip through
		// ToolName and IsNotification without panicking either —
		// both are called on every request the proxy handles.
		_ = req.IsNotification()
		_ = ToolName(&req)
	})
}
