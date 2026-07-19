package schema

import (
	"strings"
	"testing"
)

// nestedSchema builds a schema n levels deep of {"type":"object","properties":{"a":<nested>}}.
func nestedSchema(n int) *Schema {
	s := &Schema{Type: "object"}
	cur := s
	for i := 0; i < n; i++ {
		next := &Schema{Type: "object"}
		cur.Properties = map[string]*Schema{"a": next}
		cur = next
	}
	return s
}

// nestedValue builds a matching value n levels deep of {"a":{"a":{...}}}.
func nestedValue(n int) any {
	var v any = map[string]any{}
	for i := 0; i < n; i++ {
		v = map[string]any{"a": v}
	}
	return v
}

func TestValidateBoundsRecursionDepth(t *testing.T) {
	// A schema/value pair nested far beyond any real MCP tool schema. This
	// must return a depth-limit violation, not recurse until the goroutine
	// stack overflows — a malicious upstream server controls the schema a
	// Validator learns from tools/list, so this is directly attacker-reachable.
	const depth = 10000
	err := nestedSchema(depth).Validate(nestedValue(depth))
	if err == nil {
		t.Fatal("Validate: want a depth-limit error for 10000 levels of nesting, got nil")
	}
	if !strings.Contains(err.Error(), "depth") {
		t.Fatalf("Validate error = %v, want it to mention the depth limit", err)
	}
}

func TestValidateAllowsReasonableRealWorldDepth(t *testing.T) {
	// Real MCP tool schemas nest at most a handful of levels. The depth
	// guard must not fire on legitimate input.
	const depth = 10
	if err := nestedSchema(depth).Validate(nestedValue(depth)); err != nil {
		t.Fatalf("Validate: %v, want nil for a realistic %d-level schema", err, depth)
	}
}
