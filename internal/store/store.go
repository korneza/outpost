// Package store is Outpost's embedded local state layer: pinned tool-
// definition hashes, drift history, circuit-breaker state, and streaming
// anomaly aggregates, in a single SQLite file. Nothing in this package
// ever leaves the process it runs in — see internal/report for the
// separate, metadata-only contract with the hosted control plane.
package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store wraps a single SQLite database file holding all of Outpost's local
// state.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS tool_pins (
    upstream    TEXT NOT NULL,
    tool_name   TEXT NOT NULL,
    schema_hash TEXT NOT NULL,
    first_seen  TEXT NOT NULL,
    PRIMARY KEY (upstream, tool_name)
);
CREATE TABLE IF NOT EXISTS drift_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    upstream    TEXT NOT NULL,
    tool_name   TEXT NOT NULL,
    old_hash    TEXT NOT NULL,
    new_hash    TEXT NOT NULL,
    detected_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS breaker_state (
    upstream      TEXT NOT NULL,
    tool_name     TEXT NOT NULL,
    failure_count INTEGER NOT NULL DEFAULT 0,
    success_count INTEGER NOT NULL DEFAULT 0,
    state         TEXT NOT NULL DEFAULT 'closed',
    updated_at    TEXT NOT NULL,
    PRIMARY KEY (upstream, tool_name)
);
CREATE TABLE IF NOT EXISTS anomaly_aggregates (
    upstream   TEXT NOT NULL,
    tool_name  TEXT NOT NULL,
    metric     TEXT NOT NULL,
    count      INTEGER NOT NULL DEFAULT 0,
    mean       REAL NOT NULL DEFAULT 0,
    m2         REAL NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (upstream, tool_name, metric)
);
`

// Open opens (creating if necessary) the SQLite database at path and
// ensures its schema is up to date. Open is idempotent — calling it again
// against an existing file is safe and does not lose data.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}
