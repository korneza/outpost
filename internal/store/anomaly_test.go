package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSaveAndGetAnomalyAggregate(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)

	if err := s.SaveAnomalyAggregate(ctx, AnomalyAggregate{
		Upstream: "files", ToolName: "files.read", Metric: "latency_ms",
		Count: 100, Mean: 42.5, M2: 1200.75, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveAnomalyAggregate: %v", err)
	}

	agg, err := s.GetAnomalyAggregate(ctx, "files", "files.read", "latency_ms")
	if err != nil {
		t.Fatalf("GetAnomalyAggregate: %v", err)
	}
	if agg.Count != 100 || agg.Mean != 42.5 || agg.M2 != 1200.75 {
		t.Fatalf("AnomalyAggregate = %+v, want Count=100 Mean=42.5 M2=1200.75", agg)
	}
}

func TestAnomalyAggregatesAreScopedPerMetric(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)

	if err := s.SaveAnomalyAggregate(ctx, AnomalyAggregate{
		Upstream: "files", ToolName: "files.read", Metric: "latency_ms", Count: 100, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("save latency_ms: %v", err)
	}
	if err := s.SaveAnomalyAggregate(ctx, AnomalyAggregate{
		Upstream: "files", ToolName: "files.read", Metric: "call_frequency", Count: 7, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("save call_frequency: %v", err)
	}

	latency, err := s.GetAnomalyAggregate(ctx, "files", "files.read", "latency_ms")
	if err != nil {
		t.Fatalf("get latency_ms: %v", err)
	}
	freq, err := s.GetAnomalyAggregate(ctx, "files", "files.read", "call_frequency")
	if err != nil {
		t.Fatalf("get call_frequency: %v", err)
	}
	if latency.Count != 100 || freq.Count != 7 {
		t.Fatalf("latency.Count=%d freq.Count=%d, want 100 and 7 — metrics must not collide", latency.Count, freq.Count)
	}
}

func TestGetAnomalyAggregateReturnsErrNotFoundForUnknownMetric(t *testing.T) {
	s := openTestStore(t)
	_, err := s.GetAnomalyAggregate(context.Background(), "files", "files.read", "never_recorded")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
