// Package report defines Outpost's wire contract with the hosted control
// plane: the only data types that may ever cross that boundary. Per
// ADR-0001, that's SHA-256 hashes, tool definitions (near-public metadata,
// not customer data), and aggregated numeric statistics — never call
// arguments, results, or credentials. report_test.go enforces this with a
// reflection-based allowlist, not just this comment.
//
// This package defines the contract only. The HTTP client that actually
// sends these types to the control plane is Week 2 scope.
package report

import (
	"encoding/json"
	"time"
)

// PinEvent reports a tool definition's hash as first observed, or as
// changed. ToolDef is the tool definition itself — near-public metadata by
// design (ADR-0001), never the arguments or results of calling that tool.
type PinEvent struct {
	Upstream   string          `json:"upstream"`
	ToolName   string          `json:"tool_name"`
	SchemaHash string          `json:"schema_hash"`
	ToolDef    json.RawMessage `json:"tool_def"`
	DetectedAt time.Time       `json:"detected_at"`
}

// DriftEvent reports that a tool definition's hash changed.
type DriftEvent struct {
	Upstream   string    `json:"upstream"`
	ToolName   string    `json:"tool_name"`
	OldHash    string    `json:"old_hash"`
	NewHash    string    `json:"new_hash"`
	DetectedAt time.Time `json:"detected_at"`
}

// StatSnapshot reports aggregated numeric statistics for one metric over
// one time window — never individual call data.
type StatSnapshot struct {
	Upstream    string    `json:"upstream"`
	ToolName    string    `json:"tool_name"`
	Metric      string    `json:"metric"`
	Count       int64     `json:"count"`
	Mean        float64   `json:"mean"`
	P50         float64   `json:"p50"`
	P99         float64   `json:"p99"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
}
