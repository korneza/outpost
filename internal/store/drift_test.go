package store

import (
	"context"
	"testing"
	"time"
)

func TestRecordAndListDrift(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	t1 := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	if err := s.RecordDrift(ctx, DriftEvent{
		Upstream: "files", ToolName: "files.read", OldHash: "hash-a", NewHash: "hash-b", DetectedAt: t1,
	}); err != nil {
		t.Fatalf("RecordDrift 1: %v", err)
	}
	if err := s.RecordDrift(ctx, DriftEvent{
		Upstream: "files", ToolName: "files.read", OldHash: "hash-b", NewHash: "hash-c", DetectedAt: t2,
	}); err != nil {
		t.Fatalf("RecordDrift 2: %v", err)
	}

	events, err := s.ListDrift(ctx, "files", "files.read")
	if err != nil {
		t.Fatalf("ListDrift: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[0].OldHash != "hash-a" || events[0].NewHash != "hash-b" {
		t.Errorf("events[0] = %+v, want OldHash=hash-a NewHash=hash-b", events[0])
	}
	if events[1].OldHash != "hash-b" || events[1].NewHash != "hash-c" {
		t.Errorf("events[1] = %+v, want OldHash=hash-b NewHash=hash-c", events[1])
	}
	if !events[0].DetectedAt.Equal(t1) {
		t.Errorf("events[0].DetectedAt = %v, want %v", events[0].DetectedAt, t1)
	}
}

func TestListDriftEmptyForUnknownTool(t *testing.T) {
	s := openTestStore(t)
	events, err := s.ListDrift(context.Background(), "files", "never.seen")
	if err != nil {
		t.Fatalf("ListDrift: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("len(events) = %d, want 0", len(events))
	}
}

func TestListDriftedToolsReturnsDistinctPairs(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)

	// Two drift events for the same tool must not produce a duplicate entry.
	if err := s.RecordDrift(ctx, DriftEvent{Upstream: "files", ToolName: "files.read", OldHash: "a", NewHash: "b", DetectedAt: now}); err != nil {
		t.Fatalf("RecordDrift 1: %v", err)
	}
	if err := s.RecordDrift(ctx, DriftEvent{Upstream: "files", ToolName: "files.read", OldHash: "b", NewHash: "c", DetectedAt: now}); err != nil {
		t.Fatalf("RecordDrift 2: %v", err)
	}
	if err := s.RecordDrift(ctx, DriftEvent{Upstream: "files", ToolName: "files.write", OldHash: "x", NewHash: "y", DetectedAt: now}); err != nil {
		t.Fatalf("RecordDrift 3: %v", err)
	}

	tools, err := s.ListDriftedTools(ctx)
	if err != nil {
		t.Fatalf("ListDriftedTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("len(tools) = %d, want 2 distinct (upstream, tool) pairs, got %+v", len(tools), tools)
	}
	seen := map[string]bool{}
	for _, dt := range tools {
		seen[dt.Upstream+"|"+dt.ToolName] = true
	}
	if !seen["files|files.read"] || !seen["files|files.write"] {
		t.Fatalf("tools = %+v, want files.read and files.write both present", tools)
	}
}

func TestListDriftedToolsEmptyWhenNoDrift(t *testing.T) {
	s := openTestStore(t)
	tools, err := s.ListDriftedTools(context.Background())
	if err != nil {
		t.Fatalf("ListDriftedTools: %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("len(tools) = %d, want 0", len(tools))
	}
}
