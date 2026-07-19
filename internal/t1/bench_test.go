package t1

import (
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/korneza/outpost/internal/mcp"
)

func newLearnedValidator() (*Validator, *mcp.Request) {
	v := New()
	v.LearnFromToolsList(&mcp.Response{Result: json.RawMessage(filesReadListResponse)})
	req := &mcp.Request{Method: mcp.MethodToolsCall, Params: json.RawMessage(`{"name":"files.read","arguments":{"path":"/tmp/x"}}`)}
	return v, req
}

// TestCheckP99UnderOneMillisecond is the CI-enforced version of the 30-day
// plan's "T1 overhead p99 < 1ms" exit criterion, measured directly rather
// than left to a benchmark a human has to read.
func TestCheckP99UnderOneMillisecond(t *testing.T) {
	v, req := newLearnedValidator()

	const n = 2000
	durations := make([]time.Duration, n)
	for i := 0; i < n; i++ {
		start := time.Now()
		v.Check("files.read", req)
		durations[i] = time.Since(start)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p50 := durations[n/2]
	p99 := durations[int(float64(n)*0.99)]
	t.Logf("T1 Check latency over %d calls: p50=%v p99=%v", n, p50, p99)
	if p99 > time.Millisecond {
		t.Fatalf("p99 = %v, want < 1ms", p99)
	}
}

func BenchmarkCheck(b *testing.B) {
	v, req := newLearnedValidator()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Check("files.read", req)
	}
}
