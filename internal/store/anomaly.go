package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// AnomalyAggregate holds Welford's online algorithm's running accumulators
// (Count, Mean, M2) for one (upstream, tool, metric) triple, so streaming
// mean/variance can be updated incrementally without replaying history.
type AnomalyAggregate struct {
	Upstream  string
	ToolName  string
	Metric    string
	Count     int64
	Mean      float64
	M2        float64
	UpdatedAt time.Time
}

// SaveAnomalyAggregate upserts the aggregate for (Upstream, ToolName, Metric).
func (s *Store) SaveAnomalyAggregate(ctx context.Context, agg AnomalyAggregate) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO anomaly_aggregates (upstream, tool_name, metric, count, mean, m2, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (upstream, tool_name, metric) DO UPDATE SET
			count = excluded.count, mean = excluded.mean, m2 = excluded.m2, updated_at = excluded.updated_at
	`, agg.Upstream, agg.ToolName, agg.Metric, agg.Count, agg.Mean, agg.M2, agg.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: save anomaly aggregate: %w", err)
	}
	return nil
}

// GetAnomalyAggregate looks up the aggregate for (upstream, toolName,
// metric), or ErrNotFound if nothing has been recorded for that metric yet.
func (s *Store) GetAnomalyAggregate(ctx context.Context, upstream, toolName, metric string) (*AnomalyAggregate, error) {
	var agg AnomalyAggregate
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT upstream, tool_name, metric, count, mean, m2, updated_at FROM anomaly_aggregates
		WHERE upstream = ? AND tool_name = ? AND metric = ?
	`, upstream, toolName, metric).Scan(&agg.Upstream, &agg.ToolName, &agg.Metric, &agg.Count, &agg.Mean, &agg.M2, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get anomaly aggregate: %w", err)
	}
	agg.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse updated_at: %w", err)
	}
	return &agg, nil
}
