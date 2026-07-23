package stdioupstream

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/korneza/outpost/internal/mcp"
)

func buildFixture(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "fixture.exe") // .exe suffix is required on Windows (go build always appends it there) and harmless elsewhere
	cmd := exec.Command("go", "build", "-o", bin, "./testdata/fixture")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v\n%s", err, out)
	}
	return bin
}

func buildHostileFixture(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "hostile.exe")
	cmd := exec.Command("go", "build", "-o", bin, "./testdata/hostile")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build hostile fixture: %v\n%s", err, out)
	}
	return bin
}

func TestCallRoundTripsThroughChildProcess(t *testing.T) {
	bin := buildFixture(t)
	c, err := New(bin)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call", Params: json.RawMessage(`{"name":"echo","arguments":{}}`)}
	resp, err := c.Call(context.Background(), mcp.VersionCurrent, req, "")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result["content"] != "fixture response for tools/call" {
		t.Fatalf("content = %q, want fixture echo", result["content"])
	}
}

func TestCallSerializesConcurrentRequestsWithoutCorruption(t *testing.T) {
	bin := buildFixture(t)
	c, err := New(bin)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(_ int) {
			req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "tools/call"}
			_, err := c.Call(context.Background(), mcp.VersionCurrent, req, "")
			done <- err
		}(i)
	}
	for i := 0; i < 10; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent call %d: %v", i, err)
		}
	}
}

func TestNewReturnsErrorForNonexistentCommand(t *testing.T) {
	_, err := New(filepath.Join(t.TempDir(), "this-binary-does-not-exist"))
	if err == nil {
		t.Fatal("expected an error spawning a nonexistent command")
	}
}

func TestCallDoesNotReadAResponseForANotification(t *testing.T) {
	bin := buildFixture(t)
	c, err := New(bin)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	// No ID field at all — this is what makes it a notification per
	// mcp.Request.IsNotification (len(ID) == 0). The fixture is
	// spec-compliant (skips responding to notifications), so Call must
	// return immediately without blocking on a read that will never
	// come.
	notification := &mcp.Request{JSONRPC: "2.0", Method: "notifications/initialized"}
	resp, err := c.Call(context.Background(), mcp.VersionCurrent, notification, "")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp != nil {
		t.Fatalf("resp = %+v, want nil for a notification", resp)
	}

	// A real request afterward must get its own real response, not
	// anything left over from the notification above.
	req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call"}
	resp, err = c.Call(context.Background(), mcp.VersionCurrent, req, "")
	if err != nil {
		t.Fatalf("Call after notification: %v", err)
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result["content"] != "fixture response for tools/call" {
		t.Fatalf("content = %q, want the real call's own response", result["content"])
	}
}

func TestCallReturnsErrorWhenChildExitsWithoutResponding(t *testing.T) {
	bin := buildHostileFixture(t)
	c, err := New(bin)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call"}
	_, err = c.Call(context.Background(), mcp.VersionCurrent, req, "")
	if err == nil {
		t.Fatal("expected an error when the child process exits without ever responding")
	}
}

func TestCallReturnsErrorOnMalformedChildResponse(t *testing.T) {
	bin := buildHostileFixture(t)
	c, err := New(bin, "-garbage")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call"}
	_, err = c.Call(context.Background(), mcp.VersionCurrent, req, "")
	if err == nil {
		t.Fatal("expected an error when the child sends a non-JSON response")
	}
}

// TestCallRejectsResponseWithMismatchedID guards against Claude
// Security findings F3/F4/F18: Call used to trust whatever line it read
// next as the answer to the request it just sent, with no check that
// the response's JSON-RPC id actually matched. A hostile child planting
// a forged response (e.g. one sent for a notification the caller never
// reads, landing on the next unrelated call) could have its fabricated
// content returned as if it were genuine — corrupting the breaker's
// success/failure accounting downstream, among other things. The fix
// must surface this as a clear protocol error, never silently hand back
// the mismatched content.
func TestCallRejectsResponseWithMismatchedID(t *testing.T) {
	bin := buildHostileFixture(t)
	c, err := New(bin, "-wrongid")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call"}
	resp, err := c.Call(context.Background(), mcp.VersionCurrent, req, "")
	if err == nil {
		t.Fatalf("expected an error for a response id mismatch, got a response: %+v", resp)
	}
	if resp != nil {
		t.Fatalf("resp = %+v, want nil — a mismatched response must never be handed back as the answer", resp)
	}
}
