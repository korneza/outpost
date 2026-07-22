package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPrintsUsageAndExits2WithNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"outpost"}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "Usage") {
		t.Fatalf("stderr = %q, want it to contain usage text", stderr.String())
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"outpost", "bogus"}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), `unknown command "bogus"`) {
		t.Fatalf("stderr = %q, want it to name the unknown command", stderr.String())
	}
}

func TestRunVersionPrintsVersionAndExits0(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"outpost", "version"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0, stderr = %s", code, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) == "" {
		t.Fatal("expected version output on stdout")
	}
}

func TestRunSubcommandRequiresDashDashSeparator(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"outpost", "run", "some-command"}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "requires -- <server-cmd>") {
		t.Fatalf("stderr = %q, want the -- usage message", stderr.String())
	}
}

func TestRunSubcommandRejectsDashDashWithNothingAfter(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"outpost", "run", "--"}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "requires -- <server-cmd>") {
		t.Fatalf("stderr = %q, want the -- usage message", stderr.String())
	}
}

func TestRunSubcommandDispatchesToStdioWrapper(t *testing.T) {
	fixtureBin := filepath.Join(t.TempDir(), "fixture.exe")
	build := exec.Command("go", "build", "-o", fixtureBin, "../../internal/stdioupstream/testdata/fixture")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = os.Remove("outpost.db") }) // run()'s "run" case hardcodes this relative path

	var stdout, stderr bytes.Buffer
	code := run([]string{"outpost", "run", "--", fixtureBin}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0, stderr = %s", code, stderr.String())
	}
}

func TestRunServeSubcommandReturnsErrorExitCodeOnBadConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"outpost", "serve", "-config", filepath.Join(t.TempDir(), "does-not-exist.yaml")}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1, stderr = %s", code, stderr.String())
	}
}
