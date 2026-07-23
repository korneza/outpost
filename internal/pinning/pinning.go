// Package pinning implements Outpost's tool-definition pinning and drift
// detection: SHA-256 hash every tool definition on first sight (trust-on-
// first-use), and flag it when a later sighting's hash differs.
//
// The hash covers the entire tool definition — name, description,
// inputSchema, everything — not just inputSchema. A prompt-injection
// ("tool poisoning") attack typically rewrites a tool's description to
// embed hidden instructions while leaving inputSchema untouched; T1's
// schema-only view would never see that change, so pinning has to look at
// the whole object.
package pinning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/korneza/outpost/internal/mcp"
	"github.com/korneza/outpost/internal/store"
)

// DriftAlert reports that a tool's definition hash changed since it was
// pinned.
type DriftAlert struct {
	Upstream string
	ToolName string
	OldHash  string
	NewHash  string
	ToolDef  json.RawMessage
}

// Pinner tracks tool-definition hashes and detects drift. A Pinner is safe
// for concurrent use.
type Pinner struct {
	store *store.Store

	mu      sync.RWMutex
	drifted map[string]bool // key: upstream|tool
}

// New returns a Pinner backed by st.
func New(st *store.Store) *Pinner {
	return &Pinner{store: st, drifted: make(map[string]bool)}
}

func key(upstream, tool string) string {
	return upstream + "|" + tool
}

type toolsListResult struct {
	Tools []json.RawMessage `json:"tools"`
}

// LearnFromToolsList extracts each tool's full definition from a
// successful tools/list response, pins any not seen before, and reports a
// DriftAlert for any whose hash no longer matches its pin. A resp with an
// error, no result, or malformed tools are silently skipped — learning is
// best-effort and must never itself cause a failure.
func (p *Pinner) LearnFromToolsList(ctx context.Context, upstream string, resp *mcp.Response) ([]DriftAlert, error) {
	if resp == nil || resp.Error != nil || len(resp.Result) == 0 {
		return nil, nil
	}
	var result toolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, nil
	}

	var alerts []DriftAlert
	for _, raw := range result.Tools {
		var named struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &named); err != nil || named.Name == "" {
			continue
		}

		hash, err := canonicalHash(raw)
		if err != nil {
			continue
		}

		existing, err := p.store.GetPin(ctx, upstream, named.Name)
		if errors.Is(err, store.ErrNotFound) {
			if _, err := p.store.CreatePinIfAbsent(ctx, store.ToolPin{
				Upstream: upstream, ToolName: named.Name, SchemaHash: hash, FirstSeen: time.Now(),
			}); err != nil {
				return alerts, fmt.Errorf("pinning: create pin for %s: %w", named.Name, err)
			}
			continue
		}
		if err != nil {
			return alerts, fmt.Errorf("pinning: get pin for %s: %w", named.Name, err)
		}

		if existing.SchemaHash == hash {
			continue
		}

		if err := p.store.RecordDrift(ctx, store.DriftEvent{
			Upstream: upstream, ToolName: named.Name,
			OldHash: existing.SchemaHash, NewHash: hash, DetectedAt: time.Now(),
		}); err != nil {
			return alerts, fmt.Errorf("pinning: record drift for %s: %w", named.Name, err)
		}
		p.mu.Lock()
		p.drifted[key(upstream, named.Name)] = true
		p.mu.Unlock()
		alerts = append(alerts, DriftAlert{Upstream: upstream, ToolName: named.Name, OldHash: existing.SchemaHash, NewHash: hash, ToolDef: raw})
	}
	return alerts, nil
}

// Hydrate loads existing drift history from the store and marks every
// (upstream, tool) pair with at least one recorded drift event as
// currently drifted. Call this once after New, before serving traffic —
// without it, a process restart silently clears in-memory block state
// (IsDrifted) even though the persisted pin and drift log are untouched,
// which would let a previously block:true-blocked tool become callable
// again until the next tools/list happens to re-detect the same drift.
// There is no "resolve drift" mechanism in v1, so any recorded drift is
// treated as still active — the conservative, safe direction.
func (p *Pinner) Hydrate(ctx context.Context) error {
	tools, err := p.store.ListDriftedTools(ctx)
	if err != nil {
		return fmt.Errorf("pinning: hydrate: %w", err)
	}
	p.mu.Lock()
	for _, t := range tools {
		p.drifted[key(t.Upstream, t.ToolName)] = true
	}
	p.mu.Unlock()
	return nil
}

// IsDrifted reports whether (upstream, tool) currently has an unresolved
// drift alert. False for a tool with no history, or whose most recent
// sighting matched its pin.
func (p *Pinner) IsDrifted(upstream, tool string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.drifted[key(upstream, tool)]
}

// driftRedactionNotice replaces a drifted tool's description in
// RedactDrifted's output.
const driftRedactionNotice = "[Outpost] This tool's definition has changed since it was first pinned and has not been reviewed. Its description has been withheld pending review."

// RedactDrifted rewrites resp's tools/list result, replacing the
// description of every tool currently flagged as drifted (IsDrifted)
// with a fixed notice instead of forwarding it to the caller. Detecting
// drift without doing this is nearly pointless: a tool's description is
// exactly where a prompt-injection/tool-poisoning attack lives (see this
// package's doc comment), and logging the drift alone never stopped the
// poisoned text from reaching the calling agent on this or any later
// tools/list call — only a later tools/call to that specific tool could
// be blocked, and only when block: true is configured for it. Only the
// description is replaced; name and inputSchema are left intact so the
// tool stays functionally callable. A resp with an error, no result, or
// malformed tools is returned unchanged.
func (p *Pinner) RedactDrifted(upstream string, resp *mcp.Response) *mcp.Response {
	if resp == nil || resp.Error != nil || len(resp.Result) == 0 {
		return resp
	}
	var result toolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return resp
	}

	changed := false
	for i, raw := range result.Tools {
		var named struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &named); err != nil || named.Name == "" {
			continue
		}
		if !p.IsDrifted(upstream, named.Name) {
			continue
		}
		var tool map[string]json.RawMessage
		if err := json.Unmarshal(raw, &tool); err != nil {
			continue
		}
		noticeJSON, err := json.Marshal(driftRedactionNotice)
		if err != nil {
			continue
		}
		tool["description"] = noticeJSON
		redacted, err := json.Marshal(tool)
		if err != nil {
			continue
		}
		result.Tools[i] = redacted
		changed = true
	}
	if !changed {
		return resp
	}
	newResult, err := json.Marshal(result)
	if err != nil {
		return resp
	}
	out := *resp
	out.Result = newResult
	return &out
}

// canonicalHash returns the hex-encoded SHA-256 hash of raw's canonical
// form: decoded to a generic value and re-marshaled, which sorts object
// keys (a documented encoding/json behavior for map[string]any) so
// semantically-identical JSON with different key order hashes the same.
func canonicalHash(raw json.RawMessage) (string, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", fmt.Errorf("pinning: decode for hashing: %w", err)
	}
	canonical, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("pinning: re-encode for hashing: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}
