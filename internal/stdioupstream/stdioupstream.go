// Package stdioupstream implements a caller (see internal/proxy) that
// forwards MCP calls to a child process over stdio instead of HTTP —
// backs `outpost run -- <server-cmd>`'s stdio wrapper mode. MCP's stdio
// transport frames messages as newline-delimited JSON with no embedded
// newlines, matched here on both the write and read side.
//
// Calls are serialized with a mutex rather than correlated by JSON-RPC
// id — a deliberate "first cut" simplification (see the 30-day plan's
// Week 4 stretch-item framing for this feature): one child process
// backing one outpost run invocation doesn't need concurrent in-flight
// request support to be useful, and correlating by id to support that
// safely is a real, separate piece of work.
//
// Known limitation, documented rather than silently accepted: Call skips
// reading a response for a notification (correct per JSON-RPC 2.0 — a
// spec-compliant server never sends one). A non-compliant or actively
// hostile child that responds to a notification anyway would leave a
// stray line buffered, which the next real Call would then misread as
// its own response. Not defended against — the child process being
// wrapped is real, untrusted input (matching this product's own threat
// model), but full desync recovery is out of scope for this first cut.
package stdioupstream

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/korneza/outpost/internal/mcp"
)

type Caller struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Scanner

	mu sync.Mutex
}

// New spawns command (with args) and returns a Caller wired to its
// stdin/stdout. The child is started immediately; call Close when done.
func New(command string, args ...string) (*Caller, error) {
	cmd := exec.Command(command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdioupstream: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdioupstream: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("stdioupstream: start %s: %w", command, err)
	}
	return &Caller{cmd: cmd, stdin: stdin, reader: bufio.NewScanner(stdout)}, nil
}

// Call writes req as one newline-delimited JSON line to the child's
// stdin and reads the matching response line from its stdout. version
// and authHeader are accepted to satisfy proxy's caller interface but
// unused here — a stdio child process has no protocol-version header or
// bearer-token concept; MCP's stdio transport negotiates version during
// the initialize handshake carried in-band as a regular JSON-RPC call.
func (c *Caller) Call(_ context.Context, _ mcp.ProtocolVersion, req *mcp.Request, _ string) (*mcp.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("stdioupstream: encode request: %w", err)
	}
	if _, err := c.stdin.Write(append(body, '\n')); err != nil {
		return nil, fmt.Errorf("stdioupstream: write to child: %w", err)
	}

	if req.IsNotification() {
		return nil, nil // no response expected
	}

	if !c.reader.Scan() {
		if err := c.reader.Err(); err != nil {
			return nil, fmt.Errorf("stdioupstream: read from child: %w", err)
		}
		return nil, fmt.Errorf("stdioupstream: child closed stdout unexpectedly")
	}
	var resp mcp.Response
	if err := json.Unmarshal(c.reader.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("stdioupstream: decode child response: %w", err)
	}
	return &resp, nil
}

// Close closes the child's stdin (signaling EOF) and waits for it to
// exit.
func (c *Caller) Close() error {
	c.stdin.Close()
	return c.cmd.Wait()
}
