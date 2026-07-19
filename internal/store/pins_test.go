package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCreatePinIfAbsentCreatesNewPin(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	firstSeen := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)

	pin, err := s.CreatePinIfAbsent(ctx, ToolPin{
		Upstream: "files", ToolName: "files.read", SchemaHash: "abc123", FirstSeen: firstSeen,
	})
	if err != nil {
		t.Fatalf("CreatePinIfAbsent: %v", err)
	}
	if pin.SchemaHash != "abc123" {
		t.Fatalf("SchemaHash = %q, want %q", pin.SchemaHash, "abc123")
	}
	if !pin.FirstSeen.Equal(firstSeen) {
		t.Fatalf("FirstSeen = %v, want %v", pin.FirstSeen, firstSeen)
	}
}

func TestCreatePinIfAbsentDoesNotOverwriteExistingHash(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	firstSeen := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)

	if _, err := s.CreatePinIfAbsent(ctx, ToolPin{
		Upstream: "files", ToolName: "files.read", SchemaHash: "original-hash", FirstSeen: firstSeen,
	}); err != nil {
		t.Fatalf("first CreatePinIfAbsent: %v", err)
	}

	pin, err := s.CreatePinIfAbsent(ctx, ToolPin{
		Upstream: "files", ToolName: "files.read", SchemaHash: "different-hash", FirstSeen: firstSeen.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("second CreatePinIfAbsent: %v", err)
	}
	if pin.SchemaHash != "original-hash" {
		t.Fatalf("SchemaHash = %q, want the original %q — pinning must be write-once at the store layer", pin.SchemaHash, "original-hash")
	}
}

func TestGetPinReturnsErrNotFoundForUnknownTool(t *testing.T) {
	s := openTestStore(t)
	_, err := s.GetPin(context.Background(), "files", "does.not.exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
