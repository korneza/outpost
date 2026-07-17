package mcp

import (
	"encoding/json"
	"testing"
)

func TestNegotiateVersion(t *testing.T) {
	cases := []struct {
		header string
		want   ProtocolVersion
	}{
		{"", VersionCurrent},
		{"2025-11-25", VersionCurrent},
		{"2026-07-28", VersionNext},
		{"garbage", VersionCurrent},
	}
	for _, c := range cases {
		if got := NegotiateVersion(c.header); got != c.want {
			t.Errorf("NegotiateVersion(%q) = %q, want %q", c.header, got, c.want)
		}
	}
}

func TestToolNameFromToolsCall(t *testing.T) {
	req := &Request{
		Method: MethodToolsCall,
		Params: json.RawMessage(`{"name":"files.read","arguments":{"path":"/x"}}`),
	}
	if got := ToolName(req); got != "files.read" {
		t.Fatalf("ToolName = %q, want %q", got, "files.read")
	}
}

func TestToolNameEmptyForNonToolsCall(t *testing.T) {
	req := &Request{
		Method: MethodToolsList,
		Params: json.RawMessage(`{}`),
	}
	if got := ToolName(req); got != "" {
		t.Fatalf("ToolName = %q, want empty for method %q", got, MethodToolsList)
	}
}

func TestToolNameEmptyForMalformedParams(t *testing.T) {
	req := &Request{
		Method: MethodToolsCall,
		Params: json.RawMessage(`not json`),
	}
	if got := ToolName(req); got != "" {
		t.Fatalf("ToolName = %q, want empty for malformed params", got)
	}
}
