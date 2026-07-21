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
