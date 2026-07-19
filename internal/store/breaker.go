package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// BreakerState is the circuit breaker's live per-tool counters. Unlike a
// pin, this is mutable running state — SaveBreakerState always overwrites.
type BreakerState struct {
	Upstream     string
	ToolName     string
	FailureCount int
	SuccessCount int
	State        string // "closed" | "open" | "half_open"
	UpdatedAt    time.Time
}

// SaveBreakerState upserts the breaker state for (Upstream, ToolName).
func (s *Store) SaveBreakerState(ctx context.Context, bs BreakerState) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO breaker_state (upstream, tool_name, failure_count, success_count, state, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (upstream, tool_name) DO UPDATE SET
			failure_count = excluded.failure_count,
			success_count = excluded.success_count,
			state = excluded.state,
			updated_at = excluded.updated_at
	`, bs.Upstream, bs.ToolName, bs.FailureCount, bs.SuccessCount, bs.State, bs.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: save breaker state: %w", err)
	}
	return nil
}

// GetBreakerState looks up the breaker state for (upstream, toolName), or
// ErrNotFound if the breaker has never recorded a call for that tool.
func (s *Store) GetBreakerState(ctx context.Context, upstream, toolName string) (*BreakerState, error) {
	var bs BreakerState
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT upstream, tool_name, failure_count, success_count, state, updated_at FROM breaker_state
		WHERE upstream = ? AND tool_name = ?
	`, upstream, toolName).Scan(&bs.Upstream, &bs.ToolName, &bs.FailureCount, &bs.SuccessCount, &bs.State, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get breaker state: %w", err)
	}
	bs.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse updated_at: %w", err)
	}
	return &bs, nil
}
