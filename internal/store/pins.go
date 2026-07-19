package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ToolPin is a tool definition's hash as first observed for one upstream.
type ToolPin struct {
	Upstream   string
	ToolName   string
	SchemaHash string
	FirstSeen  time.Time
}

// ErrNotFound is returned when a lookup finds no matching row.
var ErrNotFound = errors.New("store: not found")

// CreatePinIfAbsent inserts a new pin if one doesn't already exist for
// (Upstream, ToolName). It returns the pin as stored — either the one just
// created, or the pre-existing one if a pin was already there. This store
// layer never overwrites a pinned hash; deciding whether to accept a
// changed hash is a judgment call for the pinning feature, not something
// persistence should do implicitly.
func (s *Store) CreatePinIfAbsent(ctx context.Context, pin ToolPin) (*ToolPin, error) {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tool_pins (upstream, tool_name, schema_hash, first_seen)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (upstream, tool_name) DO NOTHING
	`, pin.Upstream, pin.ToolName, pin.SchemaHash, pin.FirstSeen.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("store: create pin: %w", err)
	}
	return s.GetPin(ctx, pin.Upstream, pin.ToolName)
}

// GetPin looks up the pin for (upstream, toolName), or ErrNotFound if none
// exists.
func (s *Store) GetPin(ctx context.Context, upstream, toolName string) (*ToolPin, error) {
	var pin ToolPin
	var firstSeen string
	err := s.db.QueryRowContext(ctx, `
		SELECT upstream, tool_name, schema_hash, first_seen FROM tool_pins
		WHERE upstream = ? AND tool_name = ?
	`, upstream, toolName).Scan(&pin.Upstream, &pin.ToolName, &pin.SchemaHash, &firstSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get pin: %w", err)
	}
	pin.FirstSeen, err = time.Parse(time.RFC3339, firstSeen)
	if err != nil {
		return nil, fmt.Errorf("store: parse first_seen: %w", err)
	}
	return &pin, nil
}
