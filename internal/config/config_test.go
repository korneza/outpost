package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Spec §10 non-negotiable: retries are per-tool opt-in only. The Config
// struct must never grow a global retry knob.
func TestConfigHasNoGlobalRetryField(t *testing.T) {
	typ := reflect.TypeOf(Config{})
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if name == "Retry" || name == "Retries" || name == "RetryPolicy" {
			t.Fatalf("Config has global retry field %q; retries must be per-tool opt-in only", name)
		}
	}
}

func TestStateDBDefaultsWhenUnset(t *testing.T) {
	cfg := writeAndLoad(t, `
listen: "127.0.0.1:8100"
upstreams:
  - name: files
    url: "http://127.0.0.1:9000/mcp"
`)
	if cfg.StateDB != "outpost.db" {
		t.Fatalf("StateDB = %q, want default %q", cfg.StateDB, "outpost.db")
	}
}

func TestStateDBHonoursExplicitValue(t *testing.T) {
	cfg := writeAndLoad(t, `
listen: "127.0.0.1:8100"
state_db: "/var/lib/outpost/state.db"
upstreams:
  - name: files
    url: "http://127.0.0.1:9000/mcp"
`)
	if cfg.StateDB != "/var/lib/outpost/state.db" {
		t.Fatalf("StateDB = %q, want %q", cfg.StateDB, "/var/lib/outpost/state.db")
	}
}

func TestRetriesDisabledByDefault(t *testing.T) {
	cfg := writeAndLoad(t, `
listen: "127.0.0.1:8100"
upstreams:
  - name: files
    url: "http://127.0.0.1:9000/mcp"
`)
	if len(cfg.Tools) != 0 {
		t.Fatalf("expected no tool overrides by default, got %d", len(cfg.Tools))
	}
}

func TestPerToolRetryOptIn(t *testing.T) {
	cfg := writeAndLoad(t, `
listen: "127.0.0.1:8100"
upstreams:
  - name: files
    url: "http://127.0.0.1:9000/mcp"
tools:
  files.read:
    retry:
      max_attempts: 3
      initial_backoff_ms: 100
`)
	ov, ok := cfg.Tools["files.read"]
	if !ok || ov.Retry == nil {
		t.Fatal("expected files.read retry override to be present")
	}
	if ov.Retry.MaxAttempts != 3 || ov.Retry.InitialBackoffMS != 100 {
		t.Fatalf("retry override mismatch: %+v", ov.Retry)
	}
}

func TestLoadRejectsInvalid(t *testing.T) {
	for name, body := range map[string]string{
		"no listen":    `upstreams: [{name: a, url: "http://x/mcp"}]`,
		"no upstreams": `listen: "127.0.0.1:8100"`,
		"bad yaml":     `listen: [`,
	} {
		dir := t.TempDir()
		p := filepath.Join(dir, "outpost.yaml")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(p); err == nil {
			t.Fatalf("%s: expected error, got nil", name)
		}
	}
}

func TestLoadReadsControlPlaneAPIKey(t *testing.T) {
	cfg := writeAndLoad(t, "listen: \"127.0.0.1:8100\"\nupstreams:\n  - name: files\n    url: \"http://x\"\ncontrol_plane_url: \"https://cp.example.com\"\ncontrol_plane_api_key: \"secret\"\n")
	if cfg.ControlPlaneAPIKey != "secret" {
		t.Fatalf("ControlPlaneAPIKey = %q, want %q", cfg.ControlPlaneAPIKey, "secret")
	}
}

func writeAndLoad(t *testing.T, body string) *Config {
	t.Helper()
	p := filepath.Join(t.TempDir(), "outpost.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}
