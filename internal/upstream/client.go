// Package upstream implements Outpost's HTTP client for speaking MCP
// Streamable HTTP to a single upstream MCP server.
package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/korneza/outpost/internal/mcp"
)

// maxResponseBytes bounds how much of an upstream's HTTP response body
// Call will buffer into memory. The configured upstream is exactly the
// kind of actor Outpost's own threat model treats as potentially
// malicious or compromised (see docs/threat-model.md) — without a cap,
// a hostile upstream streaming an arbitrarily large body forces
// unbounded memory allocation before any status or JSON check runs
// (Claude Security findings F5/F16). It's a package var, not a const,
// only so tests can shrink it instead of transferring tens of megabytes.
// 10 MiB is generous for a real tool-call result (including a modest
// embedded file or image) while still bounding the worst case.
var maxResponseBytes int64 = 10 << 20

// Client calls one upstream MCP server. A Client is safe for concurrent use.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient returns a Client targeting baseURL.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Call sends req to the upstream over Streamable HTTP, negotiated at the
// given protocol version, and returns the decoded JSON-RPC response.
//
// For VersionNext, Call sets the Mcp-Method routing header (and Mcp-Name,
// when req is a tools/call) so infrastructure in front of the upstream can
// route without inspecting the JSON-RPC body — see ADR-0002.
//
// authHeader, if non-empty, is forwarded verbatim as the outgoing
// Authorization header — Outpost forwards opaque bearer tokens between
// agent and server; it never mints, stores, or inspects them.
func (c *Client) Call(ctx context.Context, version mcp.ProtocolVersion, req *mcp.Request, authHeader string) (*mcp.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("upstream: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("upstream: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set(mcp.ProtocolVersionHeader, string(version))
	if authHeader != "" {
		httpReq.Header.Set("Authorization", authHeader)
	}
	if version == mcp.VersionNext {
		httpReq.Header.Set("Mcp-Method", req.Method)
		if name := mcp.ToolName(req); name != "" {
			httpReq.Header.Set("Mcp-Name", name)
		}
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("upstream: request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("upstream: read response: %w", err)
	}
	if int64(len(data)) > maxResponseBytes {
		return nil, fmt.Errorf("upstream: response exceeds %d byte limit", maxResponseBytes)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream: unexpected status %d: %s", resp.StatusCode, data)
	}

	var out mcp.Response
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("upstream: decode response: %w", err)
	}
	return &out, nil
}
