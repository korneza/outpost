package breaker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/korneza/outpost/internal/store"
)

// TestConcurrentAccessDoesNotRace hammers a small set of tools from many
// goroutines simultaneously — the shape of real traffic through outpost
// serve, where every in-flight request calls Allow/RecordResult from its
// own goroutine. Run with -race (see the plan's verification step).
func TestConcurrentAccessDoesNotRace(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()
	b := New(st, Config{ConsecutiveFailureThreshold: 3, CooldownPeriod: 10 * time.Millisecond})
	ctx := context.Background()

	tools := []string{"files.read", "files.write", "files.delete"}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tool := tools[i%len(tools)]
			for j := 0; j < 100; j++ {
				if b.Allow("files", tool) {
					_ = b.RecordResult(ctx, "files", tool, j%2 == 0)
				}
			}
		}(i)
	}
	wg.Wait()
	// No assertion beyond "did not race or panic" — that's the point of
	// this test; -race is what actually verifies it (see plan Step 2).
}
