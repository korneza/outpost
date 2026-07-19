package store

import (
	"context"
	"fmt"
	"time"
)

// DriftEvent records one observed change in a tool definition's hash.
type DriftEvent struct {
	ID         int64
	Upstream   string
	ToolName   string
	OldHash    string
	NewHash    string
	DetectedAt time.Time
}

// RecordDrift appends a drift event. The log is append-only — drift history
// is never edited or deleted.
func (s *Store) RecordDrift(ctx context.Context, ev DriftEvent) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO drift_events (upstream, tool_name, old_hash, new_hash, detected_at)
		VALUES (?, ?, ?, ?, ?)
	`, ev.Upstream, ev.ToolName, ev.OldHash, ev.NewHash, ev.DetectedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: record drift: %w", err)
	}
	return nil
}

// ListDrift returns every drift event for (upstream, toolName), oldest first.
func (s *Store) ListDrift(ctx context.Context, upstream, toolName string) ([]DriftEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, upstream, tool_name, old_hash, new_hash, detected_at FROM drift_events
		WHERE upstream = ? AND tool_name = ? ORDER BY id ASC
	`, upstream, toolName)
	if err != nil {
		return nil, fmt.Errorf("store: list drift: %w", err)
	}
	defer rows.Close()

	var events []DriftEvent
	for rows.Next() {
		var ev DriftEvent
		var detectedAt string
		if err := rows.Scan(&ev.ID, &ev.Upstream, &ev.ToolName, &ev.OldHash, &ev.NewHash, &detectedAt); err != nil {
			return nil, fmt.Errorf("store: scan drift: %w", err)
		}
		ev.DetectedAt, err = time.Parse(time.RFC3339, detectedAt)
		if err != nil {
			return nil, fmt.Errorf("store: parse detected_at: %w", err)
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

// DriftedTool identifies an (Upstream, ToolName) pair that has at least
// one recorded drift event.
type DriftedTool struct {
	Upstream string
	ToolName string
}

// ListDriftedTools returns every distinct (upstream, tool) pair with at
// least one recorded drift event — used to rebuild in-memory block state
// after a restart, since that state is not itself persisted (only the
// drift log is).
func (s *Store) ListDriftedTools(ctx context.Context) ([]DriftedTool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT upstream, tool_name FROM drift_events`)
	if err != nil {
		return nil, fmt.Errorf("store: list drifted tools: %w", err)
	}
	defer rows.Close()

	var tools []DriftedTool
	for rows.Next() {
		var dt DriftedTool
		if err := rows.Scan(&dt.Upstream, &dt.ToolName); err != nil {
			return nil, fmt.Errorf("store: scan drifted tool: %w", err)
		}
		tools = append(tools, dt)
	}
	return tools, rows.Err()
}
