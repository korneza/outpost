package report

import (
	"reflect"
	"strings"
	"testing"
)

// allowedFields is a positive allowlist: every JSON field name that may
// ever appear on a wire-contract type. Adding a field to one of the types
// below without adding it here fails this test — that's the point.
var allowedFields = map[string][]string{
	"PinEvent":     {"upstream", "tool_name", "schema_hash", "tool_def", "detected_at"},
	"DriftEvent":   {"upstream", "tool_name", "old_hash", "new_hash", "detected_at"},
	"StatSnapshot": {"upstream", "tool_name", "metric", "count", "mean", "p50", "p99", "window_start", "window_end"},
}

// bannedSubstrings catches payload-shaped fields even if allowedFields is
// ever edited carelessly — belt and braces on top of the allowlist above.
var bannedSubstrings = []string{
	"argument", "params", "result", "credential", "token", "secret", "password", "payload", "auth",
}

func TestWireContractTypesCarryOnlyAllowedFields(t *testing.T) {
	types := map[string]any{
		"PinEvent":     PinEvent{},
		"DriftEvent":   DriftEvent{},
		"StatSnapshot": StatSnapshot{},
	}
	for typeName, sample := range types {
		typ := reflect.TypeOf(sample)
		allowed := make(map[string]bool)
		for _, f := range allowedFields[typeName] {
			allowed[f] = true
		}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
			if jsonName == "" {
				t.Errorf("%s.%s has no json tag — every wire field must be explicitly tagged", typeName, field.Name)
				continue
			}
			if !allowed[jsonName] {
				t.Errorf("%s has field %q not in the allowlist — wire-contract types carry metadata only (ADR-0001); if this is genuinely safe, add it to allowedFields deliberately, not by accident", typeName, jsonName)
			}
			lower := strings.ToLower(jsonName)
			for _, banned := range bannedSubstrings {
				if strings.Contains(lower, banned) {
					t.Errorf("%s has field %q containing banned substring %q — this looks like it could carry call payload data across the metadata boundary", typeName, jsonName, banned)
				}
			}
		}
	}
}
