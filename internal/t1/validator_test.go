package t1

import (
	"encoding/json"
	"testing"

	"github.com/korneza/outpost/internal/mcp"
)

const filesReadListResponse = `{"tools":[{"name":"files.read","inputSchema":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}}]}`

func TestCheckFailsOpenForUnknownTool(t *testing.T) {
	v := New()
	req := &mcp.Request{Method: mcp.MethodToolsCall, Params: json.RawMessage(`{"name":"files.read","arguments":{}}`)}
	if violation := v.Check("files.read", req); violation != "" {
		t.Fatalf("Check = %q, want \"\" (fail-open) — no schema has been learned yet", violation)
	}
}

func TestLearnFromToolsListThenCheckRejectsInvalidCall(t *testing.T) {
	v := New()
	v.LearnFromToolsList(&mcp.Response{Result: json.RawMessage(filesReadListResponse)})

	req := &mcp.Request{Method: mcp.MethodToolsCall, Params: json.RawMessage(`{"name":"files.read","arguments":{}}`)}
	violation := v.Check("files.read", req)
	if violation == "" {
		t.Fatal("Check: want a violation, arguments is missing the required path field")
	}
}

func TestLearnFromToolsListThenCheckAcceptsValidCall(t *testing.T) {
	v := New()
	v.LearnFromToolsList(&mcp.Response{Result: json.RawMessage(filesReadListResponse)})

	req := &mcp.Request{Method: mcp.MethodToolsCall, Params: json.RawMessage(`{"name":"files.read","arguments":{"path":"/tmp/x"}}`)}
	if violation := v.Check("files.read", req); violation != "" {
		t.Fatalf("Check = %q, want \"\" for a valid call", violation)
	}
}

func TestLearnFromToolsListIgnoresErrorResponse(t *testing.T) {
	v := New()
	v.LearnFromToolsList(&mcp.Response{Error: &mcp.Error{Code: -32603, Message: "boom"}})

	req := &mcp.Request{Method: mcp.MethodToolsCall, Params: json.RawMessage(`{"name":"files.read","arguments":{}}`)}
	if violation := v.Check("files.read", req); violation != "" {
		t.Fatalf("Check = %q, want \"\" — an error response must not be learned from", violation)
	}
}

func TestLearnFromToolsListIgnoresMalformedResult(t *testing.T) {
	v := New()
	v.LearnFromToolsList(&mcp.Response{Result: json.RawMessage(`not json`)})

	req := &mcp.Request{Method: mcp.MethodToolsCall, Params: json.RawMessage(`{"name":"files.read","arguments":{}}`)}
	if violation := v.Check("files.read", req); violation != "" {
		t.Fatalf("Check = %q, want \"\" — malformed tools/list results must not panic or poison the cache", violation)
	}
}

func TestCheckOnUnparsableParamsReturnsViolation(t *testing.T) {
	v := New()
	v.LearnFromToolsList(&mcp.Response{Result: json.RawMessage(filesReadListResponse)})

	req := &mcp.Request{Method: mcp.MethodToolsCall, Params: json.RawMessage(`not json`)}
	if violation := v.Check("files.read", req); violation == "" {
		t.Fatal("Check: want a violation for unparsable params on a tool with a known schema")
	}
}
