package chaos

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/korneza/outpost/internal/config"
	"github.com/korneza/outpost/internal/logging"
	"github.com/korneza/outpost/internal/proxy"
	"github.com/korneza/outpost/internal/store"
)

// TestProxySurvives30SecondsOfChaos is a bounded, CI-friendly proof that
// the real chaos harness works and the proxy stays up under it — not the
// full 24-hour soak from the Week-4 plan (impractical inside a normal
// test run). Run cmd/soaktest for the real long-duration version.
func TestProxySurvives30SecondsOfChaos(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 30s chaos soak proof in -short mode")
	}
	up := Upstream(7)
	defer up.Close()

	cfg := &config.Config{
		Listen:    "127.0.0.1:0",
		StateDB:   filepath.Join(t.TempDir(), "outpost.db"),
		Upstreams: []config.Upstream{{Name: "files", URL: up.URL}},
	}
	st, err := store.Open(cfg.StateDB)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var logBuf bytes.Buffer
	handler, err := proxy.New(cfg, logging.New(&logBuf), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()

	deadline := time.Now().Add(30 * time.Second)
	client := &http.Client{Timeout: 2 * time.Second}
	calls := 0
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/files", strings.NewReader(
			`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"files.read","arguments":{}}}`,
		))
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
		}
		calls++
	}
	t.Logf("survived %d calls against a chaotic upstream over 30s without crashing", calls)

	// The proxy itself must still be answering /healthz after the chaos
	// window — proof it never wedged, even though the upstream was
	// actively hostile throughout. Retried briefly: 20k+ rapid
	// connections in the loop above (many hard-dropped by the chaos
	// upstream, forcing a fresh TCP handshake instead of a reused
	// connection) can transiently exhaust Windows' ephemeral port pool
	// right at the deadline — a client-side OS artifact of this test's
	// own aggressive connection churn, not a proxy failure. Ports free
	// up within milliseconds as the OS reclaims them.
	var body map[string]string
	var lastErr error
	healthzDeadline := time.Now().Add(20 * time.Second) // generous: Windows' default TCP TIME_WAIT is 120s, ports free up well before that in practice
	for time.Now().Before(healthzDeadline) {
		resp, err := client.Get(srv.URL + "/healthz")
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		lastErr = nil
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		break
	}
	if lastErr != nil {
		t.Fatalf("proxy stopped responding to /healthz after the chaos window (retried for 20s): %v", lastErr)
	}
	if body["status"] != "ok" {
		t.Fatalf("/healthz body = %v, want status ok", body)
	}
}
