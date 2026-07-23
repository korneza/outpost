// Package breaker implements Outpost's per-tool circuit breaker for
// tools/call: after enough consecutive failures it stops attempting calls
// to a known-broken tool until a cooldown elapses, then allows one trial
// call before fully re-closing.
//
// The hot path (Allow, RecordResult's in-memory bookkeeping) never touches
// disk — state lives in a mutex-protected map. Only state *transitions* are
// persisted to internal/store, for observability and crash visibility, not
// for cross-restart recovery (a restart resets every breaker to closed —
// a deliberate, safe default; see the circuit-breaker plan's decision log
// entry).
//
// "Fail-open" (ADR-0003) means a failure in the breaker's own persistence
// write must never change what Allow returns. It does not mean a tripped
// breaker fails to block calls — blocking calls to a known-broken tool is
// the feature working as designed.
package breaker

import (
	"context"
	"sync"
	"time"

	"github.com/korneza/outpost/internal/boundedset"
	"github.com/korneza/outpost/internal/store"
)

// maxTrackedTools bounds how many distinct (upstream, tool) pairs the
// breaker keeps state for at once. tool is client-supplied with no
// validation of its own by this package, so without a cap an attacker
// sending a fresh fabricated tool name on every request could grow
// b.tools without bound (Claude Security finding F9). 10,000 is
// generous headroom for any real deployment's actual tool count, not a
// tight operational budget.
const maxTrackedTools = 10_000

// Clock abstracts time so cooldown behavior is testable without real sleeps.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Config controls when a breaker trips and how long it stays open.
type Config struct {
	ConsecutiveFailureThreshold int
	CooldownPeriod              time.Duration
}

// DefaultConfig returns reasonable defaults: 5 consecutive failures trips
// the breaker, with a 30-second cooldown before a trial call is allowed.
func DefaultConfig() Config {
	return Config{ConsecutiveFailureThreshold: 5, CooldownPeriod: 30 * time.Second}
}

type toolState struct {
	state               string // "closed" | "open" | "half_open"
	consecutiveFailures int
	openedAt            time.Time
}

// Breaker tracks circuit-breaker state per (upstream, tool). A Breaker is
// safe for concurrent use.
type Breaker struct {
	store *store.Store
	cfg   Config
	clock Clock

	mu      sync.Mutex
	tools   map[string]*toolState
	tracked *boundedset.Tracker
}

// New returns a Breaker backed by st for transition persistence.
func New(st *store.Store, cfg Config) *Breaker {
	return &Breaker{
		store:   st,
		cfg:     cfg,
		clock:   realClock{},
		tools:   make(map[string]*toolState),
		tracked: boundedset.New(maxTrackedTools),
	}
}

func key(upstream, tool string) string {
	return upstream + "|" + tool
}

// Allow reports whether a tools/call to (upstream, tool) may proceed. A
// closed or unknown tool is always allowed. An open tool is allowed only
// once the cooldown has elapsed, at which point the breaker transitions to
// half-open and this call is the trial. A half-open tool is allowed (the
// trial in flight).
func (b *Breaker) Allow(upstream, tool string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	ts, ok := b.tools[key(upstream, tool)]
	if !ok || ts.state == "closed" {
		return true
	}
	if ts.state == "half_open" {
		return true
	}
	// state == "open"
	if b.clock.Now().Sub(ts.openedAt) >= b.cfg.CooldownPeriod {
		ts.state = "half_open"
		b.persist(upstream, tool, ts)
		return true
	}
	return false
}

// RecordResult updates breaker state for (upstream, tool) based on a
// call's outcome. A persistence error is returned for observability but
// never changes the in-memory state Allow reads from.
func (b *Breaker) RecordResult(ctx context.Context, upstream, tool string, success bool) error {
	b.mu.Lock()
	k := key(upstream, tool)
	ts, ok := b.tools[k]
	if !ok {
		ts = &toolState{state: "closed"}
		b.tools[k] = ts
		if evict, evicted := b.tracked.Add(k); evicted {
			delete(b.tools, evict)
		}
	}

	transitioned := false
	if success {
		if ts.state == "half_open" {
			ts.state = "closed"
			transitioned = true
		}
		ts.consecutiveFailures = 0
	} else {
		ts.consecutiveFailures++
		switch ts.state {
		case "closed":
			if ts.consecutiveFailures >= b.cfg.ConsecutiveFailureThreshold {
				ts.state = "open"
				ts.openedAt = b.clock.Now()
				transitioned = true
			}
		case "half_open":
			ts.state = "open"
			ts.openedAt = b.clock.Now()
			transitioned = true
		}
	}
	stateCopy := *ts
	b.mu.Unlock()

	if !transitioned {
		return nil
	}
	return b.store.SaveBreakerState(ctx, store.BreakerState{
		Upstream:     upstream,
		ToolName:     tool,
		FailureCount: stateCopy.consecutiveFailures,
		State:        stateCopy.state,
		UpdatedAt:    b.clock.Now(),
	})
}

func (b *Breaker) persist(upstream, tool string, ts *toolState) {
	// Best-effort: called with b.mu held from Allow, on the open->half_open
	// transition. A failure here must never change what Allow already
	// decided — there is deliberately no error path back to the caller.
	_ = b.store.SaveBreakerState(context.Background(), store.BreakerState{
		Upstream:     upstream,
		ToolName:     tool,
		FailureCount: ts.consecutiveFailures,
		State:        ts.state,
		UpdatedAt:    b.clock.Now(),
	})
}
