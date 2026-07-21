// Package chaos provides a deterministic, seeded chaotic HTTP upstream for
// resilience testing: slow responses, malformed bodies, and dropped
// connections, standing in for the Week-4 hardening plan's "upstream
// flaps, malformed frames, slow clients" soak-test scenarios.
package chaos

import (
	"math/rand" //nolint:gosec // reproducibility (same seed -> same sequence) is the actual requirement here, not cryptographic strength
	"net/http"
	"net/http/httptest"
	"sync"
)

// Upstream returns an httptest.Server whose handler randomly picks one of
// four behaviors per request, seeded for reproducibility: a valid
// JSON-RPC response, a malformed JSON body, a dropped connection (no
// response written at all), or a normal response representing a slow
// client's request finally completing.
func Upstream(seed int64) *httptest.Server {
	rng := rand.New(rand.NewSource(seed)) //nolint:gosec // see package-level justification above
	var mu sync.Mutex                     // math/rand.Rand isn't safe for concurrent use; net/http may invoke the handler on multiple goroutines
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		n := rng.Intn(4)
		mu.Unlock()
		switch n {
		case 0:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":"ok"}}`))
		case 1:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0",`)) // truncated/malformed
		case 2:
			hj, ok := w.(http.Hijacker)
			if !ok {
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				return
			}
			conn.Close() // drop without writing anything
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":"ok, slowly"}}`))
		}
	}))
}
