// Package mcp implements the JSON-RPC 2.0 message envelope and MCP-specific
// protocol constants used to speak to MCP servers.
package mcp

import "encoding/json"

// Request is a JSON-RPC 2.0 request or notification. A Request with a nil
// ID is a notification per the JSON-RPC 2.0 spec — see IsNotification.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// IsNotification reports whether r is a JSON-RPC notification (no id).
func (r *Request) IsNotification() bool {
	return len(r.ID) == 0
}

// Response is a JSON-RPC 2.0 response. Exactly one of Result or Error is set.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error is a JSON-RPC 2.0 error object.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// NewErrorResponse builds a JSON-RPC error Response for the given request
// id. id may be nil (e.g. when the request could not be parsed far enough
// to recover one), matching the JSON-RPC 2.0 spec's id:null convention.
func NewErrorResponse(id json.RawMessage, code int, message string) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &Error{Code: code, Message: message},
	}
}
