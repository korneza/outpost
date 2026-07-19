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
