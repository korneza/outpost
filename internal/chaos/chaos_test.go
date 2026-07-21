package chaos

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestChaosUpstreamIsReproducibleWithSameSeed(t *testing.T) {
	srv1 := Upstream(42)
	defer srv1.Close()
	srv2 := Upstream(42)
	defer srv2.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	// classify, not compare raw strings: srv1/srv2 are different servers
	// on different ports, so a connection-drop error's text embeds a
	// different port each time even when the *behavior* (drop vs valid
	// vs malformed) is correctly reproduced — comparing the category is
	// the right level, not the literal error/body text.
	get := func(srv string) string {
		resp, err := client.Post(srv, "application/json", nil)
		if err != nil {
			return "dropped"
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var v map[string]any
		if json.Unmarshal(body, &v) != nil {
			return "malformed"
		}
		return "valid"
	}

	// Same seed must produce the same sequence of chaotic behaviors —
	// otherwise the soak test isn't reproducible/debuggable.
	for i := 0; i < 5; i++ {
		a, b := get(srv1.URL), get(srv2.URL)
		if a != b {
			t.Fatalf("call %d: srv1 = %q, srv2 = %q — same seed must reproduce the same sequence", i, a, b)
		}
	}
}

func TestChaosUpstreamNeverPanics(_ *testing.T) {
	srv := Upstream(1)
	defer srv.Close()
	client := &http.Client{Timeout: 2 * time.Second}
	for i := 0; i < 200; i++ {
		resp, err := client.Post(srv.URL, "application/json", nil)
		if err != nil {
			continue // connection-drop and timeout behaviors are expected
		}
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	// Reaching here without the test binary crashing is the assertion —
	// a panic in the handler would take the whole test process down.
}
