package breaker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/korneza/outpost/internal/store"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) advance(d time.Duration) { c.now = c.now.Add(d) }

func newTestBreaker(t *testing.T, cfg Config) (*Breaker, *fakeClock) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	b := New(st, cfg)
	clock := &fakeClock{now: time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)}
	b.clock = clock
	return b, clock
}

func TestAllowStartsClosed(t *testing.T) {
	b, _ := newTestBreaker(t, DefaultConfig())
	if !b.Allow("files", "files.read") {
		t.Fatal("Allow: want true for a tool with no recorded calls yet")
	}
}

func TestTripsOpenAfterConsecutiveFailures(t *testing.T) {
	b, _ := newTestBreaker(t, Config{ConsecutiveFailureThreshold: 3, CooldownPeriod: time.Minute})
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := b.RecordResult(ctx, "files", "files.read", false); err != nil {
			t.Fatalf("RecordResult %d: %v", i, err)
		}
	}
	if b.Allow("files", "files.read") {
		t.Fatal("Allow: want false — 3 consecutive failures should have tripped the breaker open")
	}
}

func TestSuccessResetsConsecutiveFailureCount(t *testing.T) {
	b, _ := newTestBreaker(t, Config{ConsecutiveFailureThreshold: 3, CooldownPeriod: time.Minute})
	ctx := context.Background()
	_ = b.RecordResult(ctx, "files", "files.read", false)
	_ = b.RecordResult(ctx, "files", "files.read", false)
	_ = b.RecordResult(ctx, "files", "files.read", true) // resets the streak
	_ = b.RecordResult(ctx, "files", "files.read", false)
	_ = b.RecordResult(ctx, "files", "files.read", false)
	if !b.Allow("files", "files.read") {
		t.Fatal("Allow: want true — the success streak reset should have prevented tripping at only 2 consecutive failures")
	}
}

func TestHalfOpenAfterCooldownThenClosesOnSuccess(t *testing.T) {
	b, clock := newTestBreaker(t, Config{ConsecutiveFailureThreshold: 2, CooldownPeriod: 30 * time.Second})
	ctx := context.Background()
	_ = b.RecordResult(ctx, "files", "files.read", false)
	_ = b.RecordResult(ctx, "files", "files.read", false)
	if b.Allow("files", "files.read") {
		t.Fatal("Allow: want false immediately after tripping open")
	}

	clock.advance(29 * time.Second)
	if b.Allow("files", "files.read") {
		t.Fatal("Allow: want false — cooldown has not fully elapsed yet")
	}

	clock.advance(2 * time.Second) // total 31s, past the 30s cooldown
	if !b.Allow("files", "files.read") {
		t.Fatal("Allow: want true — cooldown elapsed, this should be the half-open trial call")
	}

	if err := b.RecordResult(ctx, "files", "files.read", true); err != nil {
		t.Fatalf("RecordResult (trial success): %v", err)
	}
	if !b.Allow("files", "files.read") {
		t.Fatal("Allow: want true — the trial succeeded, breaker should be closed again")
	}
}

func TestHalfOpenReopensOnTrialFailure(t *testing.T) {
	b, clock := newTestBreaker(t, Config{ConsecutiveFailureThreshold: 1, CooldownPeriod: 10 * time.Second})
	ctx := context.Background()
	_ = b.RecordResult(ctx, "files", "files.read", false) // trips open immediately (threshold 1)

	clock.advance(11 * time.Second)
	if !b.Allow("files", "files.read") {
		t.Fatal("Allow: want true for the half-open trial call")
	}
	if err := b.RecordResult(ctx, "files", "files.read", false); err != nil {
		t.Fatalf("RecordResult (trial failure): %v", err)
	}

	if b.Allow("files", "files.read") {
		t.Fatal("Allow: want false — the trial call failed, breaker should be open again, cooldown restarted")
	}
	clock.advance(11 * time.Second)
	if !b.Allow("files", "files.read") {
		t.Fatal("Allow: want true — cooldown restarted after the failed trial, and it has now elapsed again")
	}
}

// TestHalfOpenGrantsExactlyOneConcurrentTrial guards against Claude
// Security finding F21: Allow's half-open branch returned true for
// every caller once a tool was half-open, with nothing limiting the
// decision to a single in-flight trial call as the package doc implies
// — RecordResult only flips state after a call completes, so a burst
// of concurrent requests arriving right as the breaker transitions to
// half-open could all pass Allow() and hit the recovering upstream at
// once, defeating the breaker's one-trial-at-a-time recovery gate.
func TestHalfOpenGrantsExactlyOneConcurrentTrial(t *testing.T) {
	b, clock := newTestBreaker(t, Config{ConsecutiveFailureThreshold: 1, CooldownPeriod: 10 * time.Second})
	ctx := context.Background()
	_ = b.RecordResult(ctx, "files", "files.read", false) // trips open
	clock.advance(11 * time.Second)                       // cooldown elapsed

	const concurrency = 50
	allowed := make(chan bool, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed <- b.Allow("files", "files.read")
		}()
	}
	wg.Wait()
	close(allowed)

	trueCount := 0
	for a := range allowed {
		if a {
			trueCount++
		}
	}
	if trueCount != 1 {
		t.Fatalf("Allow returned true for %d of %d concurrent callers, want exactly 1 — only a single trial call may reach a recovering upstream at a time", trueCount, concurrency)
	}
}

func TestBreakerStateIsPerToolNotGlobal(t *testing.T) {
	b, _ := newTestBreaker(t, Config{ConsecutiveFailureThreshold: 1, CooldownPeriod: time.Minute})
	ctx := context.Background()
	_ = b.RecordResult(ctx, "files", "files.read", false)
	if b.Allow("files", "files.read") {
		t.Fatal("Allow: want false for the tripped tool")
	}
	if !b.Allow("files", "files.write") {
		t.Fatal("Allow: want true for a different tool on the same upstream — breaker state must not leak across tools")
	}
	if !b.Allow("other-upstream", "files.read") {
		t.Fatal("Allow: want true for the same tool name on a different upstream — breaker state must not leak across upstreams")
	}
}

// TestTrackedToolCountIsBounded guards against Claude Security finding
// F9: tool is client-supplied with no length or cardinality validation
// upstream of RecordResult, and every distinct string used to create a
// permanent map entry with no eviction — an attacker sending a fresh
// fabricated tool name per request could grow b.tools without bound.
func TestTrackedToolCountIsBounded(t *testing.T) {
	b, _ := newTestBreaker(t, DefaultConfig())
	ctx := context.Background()
	for i := 0; i < maxTrackedTools+500; i++ {
		_ = b.RecordResult(ctx, "files", "tool-"+string(rune(i)), true)
	}
	b.mu.Lock()
	n := len(b.tools)
	b.mu.Unlock()
	if n > maxTrackedTools {
		t.Fatalf("tracked tool count = %d, want capped at %d", n, maxTrackedTools)
	}
}

func TestStateTransitionsArePersisted(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()
	b := New(st, Config{ConsecutiveFailureThreshold: 1, CooldownPeriod: time.Minute})
	ctx := context.Background()

	if err := b.RecordResult(ctx, "files", "files.read", false); err != nil {
		t.Fatalf("RecordResult: %v", err)
	}

	persisted, err := st.GetBreakerState(ctx, "files", "files.read")
	if err != nil {
		t.Fatalf("GetBreakerState: %v", err)
	}
	if persisted.State != "open" {
		t.Fatalf("persisted State = %q, want %q", persisted.State, "open")
	}
}
