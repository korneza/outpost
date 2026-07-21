package t1

import (
	"encoding/json"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/korneza/outpost/internal/mcp"
)

// TestCheckP99UnderConcurrencyIsSubMillisecond is the Week-4 "concurrency
// load test of the sub-ms T1 claim, results published" item — a single-
// goroutine benchmark doesn't prove anything about real proxy load, where
// many tools/call requests hit T1.Check concurrently. This runs real
// concurrent load and asserts on the actual measured p99, not a
// single-threaded number.
func TestCheckP99UnderConcurrencyIsSubMillisecond(t *testing.T) {
	v := New()
	learnResp := &mcp.Response{Result: json.RawMessage(`{"tools":[{"name":"files.read","inputSchema":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}}]}`)}
	v.LearnFromToolsList(learnResp)

	const goroutines = 50
	const perGoroutine = 2000
	req := &mcp.Request{Method: mcp.MethodToolsCall, Params: json.RawMessage(`{"name":"files.read","arguments":{"path":"/tmp/x"}}`)}

	var mu sync.Mutex
	var latencies []time.Duration
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make([]time.Duration, 0, perGoroutine)
			for i := 0; i < perGoroutine; i++ {
				start := time.Now()
				if violation := v.Check("files.read", req); violation != "" {
					t.Errorf("unexpected violation: %s", violation)
					return
				}
				local = append(local, time.Since(start))
			}
			mu.Lock()
			latencies = append(latencies, local...)
			mu.Unlock()
		}()
	}
	wg.Wait()

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[len(latencies)*50/100]
	p99 := latencies[len(latencies)*99/100]
	t.Logf("T1.Check under %d-goroutine concurrency, %d total calls: p50=%v p99=%v", goroutines, len(latencies), p50, p99)

	if p99 >= time.Millisecond {
		t.Fatalf("p99 = %v, want < 1ms under concurrent load", p99)
	}
}
