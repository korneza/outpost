package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSaveAndGetBreakerState(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)

	if err := s.SaveBreakerState(ctx, BreakerState{
		Upstream: "files", ToolName: "files.read",
		FailureCount: 3, SuccessCount: 10, State: "closed", UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveBreakerState: %v", err)
	}

	bs, err := s.GetBreakerState(ctx, "files", "files.read")
	if err != nil {
		t.Fatalf("GetBreakerState: %v", err)
	}
	if bs.FailureCount != 3 || bs.SuccessCount != 10 || bs.State != "closed" {
		t.Fatalf("BreakerState = %+v, want FailureCount=3 SuccessCount=10 State=closed", bs)
	}
}

func TestSaveBreakerStateOverwritesPreviousState(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)

	if err := s.SaveBreakerState(ctx, BreakerState{
		Upstream: "files", ToolName: "files.read", FailureCount: 1, State: "closed", UpdatedAt: now,
	}); err != nil {
		t.Fatalf("first SaveBreakerState: %v", err)
	}
	if err := s.SaveBreakerState(ctx, BreakerState{
		Upstream: "files", ToolName: "files.read", FailureCount: 5, State: "open", UpdatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("second SaveBreakerState: %v", err)
	}

	bs, err := s.GetBreakerState(ctx, "files", "files.read")
	if err != nil {
		t.Fatalf("GetBreakerState: %v", err)
	}
	if bs.FailureCount != 5 || bs.State != "open" {
		t.Fatalf("BreakerState = %+v, want the latest save (FailureCount=5 State=open) — unlike pins, breaker state is live and must update in place", bs)
	}
}

func TestGetBreakerStateReturnsErrNotFoundForUnknownTool(t *testing.T) {
	s := openTestStore(t)
	_, err := s.GetBreakerState(context.Background(), "files", "never.seen")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestBreakerStateRowCountIsBounded guards against Claude Security
// finding F14: tool_name is client-supplied with no length or
// cardinality validation of its own at this layer, and every distinct
// (upstream, tool_name) pair persists a permanent row with no cap — an
// attacker sending tools/call with a fresh fabricated name per request
// (enough to trip the breaker each time) could grow breaker_state
// without bound. The cap is a package var here specifically so the test
// doesn't need tens of thousands of real inserts to prove the bound.
func TestBreakerStateRowCountIsBounded(t *testing.T) {
	s := openTestStore(t)
	orig := maxBreakerStateRows
	maxBreakerStateRows = 10
	t.Cleanup(func() { maxBreakerStateRows = orig })

	ctx := context.Background()
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 25; i++ {
		if err := s.SaveBreakerState(ctx, BreakerState{
			Upstream: "files", ToolName: "tool-" + string(rune(i)),
			FailureCount: 5, State: "open", UpdatedAt: now.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("SaveBreakerState %d: %v", i, err)
		}
	}

	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM breaker_state`).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count > 10 {
		t.Fatalf("breaker_state row count = %d, want capped at 10", count)
	}
}
