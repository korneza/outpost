package mcp

import "encoding/json"

// MCP JSON-RPC method names.
const (
	MethodInitialize    = "initialize"
	MethodToolsList     = "tools/list"
	MethodToolsCall     = "tools/call"
	MethodResourcesRead = "resources/read"
)

// JSON-RPC 2.0 standard error codes.
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// ProtocolVersion identifies which MCP protocol revision a message is
// negotiated for. See ADR-0002 for why both are supported concurrently.
type ProtocolVersion string

const (
	// VersionCurrent is the current stable MCP protocol.
	VersionCurrent ProtocolVersion = "2025-11-25"
	// VersionNext is the upcoming MCP protocol, tracked against its release
	// candidate until the final text ships (2026-07-28).
	VersionNext ProtocolVersion = "2026-07-28"
)

// ProtocolVersionHeader is the HTTP header Outpost uses to negotiate which
// MCP protocol version a request is speaking.
const ProtocolVersionHeader = "MCP-Protocol-Version"

// NegotiateVersion resolves the protocol version for a request from the
// incoming MCP-Protocol-Version header value. An absent or unrecognized
// header defaults to VersionCurrent — the compatibility shim for clients
// that predate this negotiation mechanism entirely.
func NegotiateVersion(header string) ProtocolVersion {
	if header == string(VersionNext) {
		return VersionNext
	}
	return VersionCurrent
}

// ToolName extracts the tool name from a tools/call request's params. It
// returns "" for any other method, or if params does not carry a name —
// callers must treat "" as "unknown/not applicable", not as an error.
func ToolName(req *Request) string {
	if req.Method != MethodToolsCall {
		return ""
	}
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return ""
	}
	return p.Name
}
