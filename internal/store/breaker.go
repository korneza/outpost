package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// maxBreakerStateRows bounds the total row count of breaker_state.
// tool_name is client-supplied with no length or cardinality validation
// of its own at this layer, and every distinct (upstream, tool_name)
// pair persists a permanent row via upsert — an attacker sending
// tools/call with a fresh fabricated name per request (enough to trip
// the breaker each time) could otherwise grow this table without bound
// (Claude Security finding F14). It's a package var, not a const, only
// so tests can shrink it instead of running tens of thousands of real
// inserts.
//
// Pruning the oldest-updated rows is safe here specifically because
// this table is observability/crash-visibility state, not used for
// cross-restart recovery (a restart resets every breaker to closed
// regardless — see this package's own doc comment) — losing an old,
// otherwise-idle tool's row costs nothing a live deployment depends on.
var maxBreakerStateRows = 20_000

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

// SaveBreakerState upserts the breaker state for (Upstream, ToolName),
// then prunes the oldest-updated rows beyond maxBreakerStateRows if this
// insertion pushed the table over the cap.
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
	if _, err := s.db.ExecContext(ctx, `
		DELETE FROM breaker_state WHERE rowid NOT IN (
			SELECT rowid FROM breaker_state ORDER BY updated_at DESC LIMIT ?
		)
	`, maxBreakerStateRows); err != nil {
		return fmt.Errorf("store: prune breaker_state: %w", err)
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
