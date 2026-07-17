package mcp

import (
	"encoding/json"
	"testing"
)

func TestRequestRoundTrip(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"files.read","arguments":{"path":"/x"}}}`
	var req Request
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if req.Method != "tools/call" {
		t.Fatalf("Method = %q, want %q", req.Method, "tools/call")
	}
	if req.IsNotification() {
		t.Fatal("expected a request with id 7 to not be a notification")
	}
	out, err := json.Marshal(&req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var roundTripped Request
	if err := json.Unmarshal(out, &roundTripped); err != nil {
		t.Fatalf("Unmarshal round-tripped: %v", err)
	}
	if string(roundTripped.ID) != "7" {
		t.Fatalf("round-tripped ID = %q, want %q", roundTripped.ID, "7")
	}
}

func TestRequestIsNotificationWhenIDAbsent(t *testing.T) {
	var req Request
	if err := json.Unmarshal([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`), &req); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !req.IsNotification() {
		t.Fatal("expected a request with no id to be a notification")
	}
}

func TestNewErrorResponse(t *testing.T) {
	id := json.RawMessage(`42`)
	resp := NewErrorResponse(id, -32601, "method not found")
	if resp.JSONRPC != "2.0" {
		t.Fatalf("JSONRPC = %q, want 2.0", resp.JSONRPC)
	}
	if string(resp.ID) != "42" {
		t.Fatalf("ID = %q, want 42", resp.ID)
	}
	if resp.Error == nil || resp.Error.Code != -32601 || resp.Error.Message != "method not found" {
		t.Fatalf("Error = %+v, want code -32601 message %q", resp.Error, "method not found")
	}
	if resp.Result != nil {
		t.Fatalf("Result = %s, want nil for an error response", resp.Result)
	}
}

func TestResponseMarshalsErrorNotResult(t *testing.T) {
	resp := NewErrorResponse(json.RawMessage(`1`), -32700, "parse error")
	out, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := decoded["result"]; ok {
		t.Fatal("error response must not include a result field")
	}
	if _, ok := decoded["error"]; !ok {
		t.Fatal("error response must include an error field")
	}
}
