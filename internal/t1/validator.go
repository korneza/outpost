// Package t1 implements Outpost's Tier-1 structural validation:
// synchronous, sub-millisecond JSON Schema checks of tools/call arguments
// against the tool's schema, learned from the upstream's own tools/list
// responses. T1 is fail-open by design (ADR-0003): a tool whose schema
// hasn't been seen yet is allowed through unchecked, never blocked.
package t1

import (
	"encoding/json"
	"sync"

	"github.com/korneza/outpost/internal/mcp"
	"github.com/korneza/outpost/internal/schema"
)

// Validator caches tool input schemas learned from tools/list responses
// and checks tools/call arguments against them. A Validator is safe for
// concurrent use.
type Validator struct {
	mu      sync.RWMutex
	schemas map[string]*schema.Schema
}

// New returns an empty Validator — nothing is rejected until schemas have
// been learned via LearnFromToolsList.
func New() *Validator {
	return &Validator{schemas: make(map[string]*schema.Schema)}
}

type toolsListResult struct {
	Tools []struct {
		Name        string          `json:"name"`
		InputSchema json.RawMessage `json:"inputSchema"`
	} `json:"tools"`
}

// LearnFromToolsList extracts each tool's inputSchema from a successful
// tools/list response and caches it for future Check calls. A resp with an
// error, no result, or tools lacking a parseable inputSchema are silently
// skipped — learning is best-effort and must never itself cause a failure.
func (v *Validator) LearnFromToolsList(resp *mcp.Response) {
	if resp == nil || resp.Error != nil || len(resp.Result) == 0 {
		return
	}
	var result toolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return
	}
	for _, tool := range result.Tools {
		if len(tool.InputSchema) == 0 {
			continue
		}
		sch, err := schema.Parse(tool.InputSchema)
		if err != nil {
			continue
		}
		v.mu.Lock()
		v.schemas[tool.Name] = sch
		v.mu.Unlock()
	}
}

// Check validates a tools/call request's arguments against the cached
// schema for toolName. It returns "" if the call may proceed — either
// because the arguments are valid, or because no schema is cached yet for
// toolName (fail-open) — and a human-readable violation summary otherwise.
func (v *Validator) Check(toolName string, req *mcp.Request) string {
	v.mu.RLock()
	sch, known := v.schemas[toolName]
	v.mu.RUnlock()
	if !known {
		return ""
	}

	var params struct {
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return "invalid params: " + err.Error()
	}

	var args any
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return "invalid arguments: " + err.Error()
		}
	}

	if err := sch.Validate(args); err != nil {
		return err.Error()
	}
	return ""
}
