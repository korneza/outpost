package main

import (
	"bufio"
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunStdioWrapperEchoesFixtureResponse(t *testing.T) {
	// Build the same stdioupstream test fixture as a stand-in "MCP
	// server" child process.
	fixtureBin := filepath.Join(t.TempDir(), "fixture.exe")
	build := exec.Command("go", "build", "-o", fixtureBin, "../../internal/stdioupstream/testdata/fixture")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v\n%s", err, out)
	}

	var stdout bytes.Buffer
	stdin := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{}}}` + "\n")

	cfg := runConfig{stateDB: filepath.Join(t.TempDir(), "outpost.db")}
	if err := runStdioWrapper(cfg, stdin, &stdout, fixtureBin, nil); err != nil {
		t.Fatalf("runStdioWrapper: %v", err)
	}

	scanner := bufio.NewScanner(&stdout)
	if !scanner.Scan() {
		t.Fatal("expected one line of output")
	}
	if !strings.Contains(scanner.Text(), "fixture response for tools/call") {
		t.Fatalf("output = %q, want it to contain the fixture's response", scanner.Text())
	}
}
