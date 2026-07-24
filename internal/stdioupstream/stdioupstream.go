// Package stdioupstream implements a caller (see internal/proxy) that
// forwards MCP calls to a child process over stdio instead of HTTP —
// backs `outpost run -- <server-cmd>`'s stdio wrapper mode. MCP's stdio
// transport frames messages as newline-delimited JSON with no embedded
// newlines, matched here on both the write and read side.
//
// Calls are serialized with a mutex rather than pipelined — a deliberate
// "first cut" simplification (see the 30-day plan's Week 4 stretch-item
// framing for this feature): one child process backing one outpost run
// invocation doesn't need concurrent in-flight requests to be useful.
//
// Call does correlate each response to the request that produced it by
// JSON-RPC id (a Claude Security scan finding, fixed 2026-07-23) — a
// mismatch is rejected as a protocol error rather than handed back as
// the answer. Call also still skips reading a response for a
// notification (correct per JSON-RPC 2.0 — a spec-compliant server
// never sends one). A non-compliant or actively hostile child that
// responds to a notification anyway leaves a stray line buffered; the
// id check stops that stray line from being trusted as a later call's
// real answer, but the underlying desync — the child's genuine next
// response is now one line further down the stream — isn't itself
// repaired here. The child process being wrapped is real, untrusted
// input (matching this product's own threat model); full self-healing
// resync is a separate, larger piece of work than closing off the
// "wrong answer accepted as right" failure mode this fixes.
package stdioupstream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/korneza/outpost/internal/mcp"
)

// curatedChildEnv returns a minimal environment for a wrapped child
// process instead of the parent's full one. Leaving exec.Cmd.Env nil
// means "inherit every environment variable of the outpost process
// verbatim" (Go's os/exec semantics) — but the wrapped command is real,
// untrusted input by this package's own stated threat model, and a
// malicious or supply-chain-compromised one could read os.Environ() and
// exfiltrate any secret sitting in whatever shell launched outpost
// (cloud keys, CONTROL_PLANE_API_KEY, CI tokens), entirely outside the
// stdio channel this package's own gate logic inspects (Claude Security
// finding F7). Only variables a child process genuinely needs to
// function at all are passed through.
func curatedChildEnv() []string {
	var env []string
	for _, name := range []string{
		"PATH",
		"HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH",
		"TEMP", "TMP", "TMPDIR",
		"SystemRoot", "SystemDrive", // Windows: DNS/network init can fail without these
	} {
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	return env
}

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
	cmd.Env = curatedChildEnv()
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
	reader := bufio.NewScanner(stdout)
	// bufio.Scanner defaults to a 64KB max token size (Claude Security
	// finding F6). The untrusted child's response line is read straight
	// from this scanner, and a real tools/call result — one embedding a
	// modest file or image, say — can plausibly exceed that. Worse,
	// Scanner's error state is sticky once a line is too long: every
	// later Scan() call keeps returning the same error, permanently
	// breaking the whole session on one oversized line rather than just
	// failing that one call. maxResponseLineBytes gives real headroom
	// instead.
	reader.Buffer(make([]byte, 0, 64*1024), maxResponseLineBytes)
	return &Caller{cmd: cmd, stdin: stdin, reader: reader}, nil
}

// maxResponseLineBytes bounds the largest single stdio response line
// Call will accept, matching internal/upstream.Client's HTTP response
// cap so both transports treat "how big can one real tool-call result
// be" consistently.
const maxResponseLineBytes = 10 << 20

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
	// Correlate by JSON-RPC id rather than trusting read order. Without
	// this, a stray line the child wrote for something else — most
	// plausibly a response to a notification, which per spec it should
	// never send and this Caller never reads — gets picked up by
	// whichever unrelated Call() scans next and returned as if it were
	// that call's genuine answer.
	if !bytes.Equal(resp.ID, req.ID) {
		return nil, fmt.Errorf("stdioupstream: response id %s does not match request id %s (child stdio desynced or sent an unsolicited response)", resp.ID, req.ID)
	}
	return &resp, nil
}

// Close closes the child's stdin (signaling EOF) and waits for it to
// exit.
func (c *Caller) Close() error {
	c.stdin.Close()
	return c.cmd.Wait()
}
