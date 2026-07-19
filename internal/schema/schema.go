// Package schema implements a minimal JSON Schema validator covering the
// subset MCP tool inputSchemas commonly use: object/array/string/number/
// integer/boolean/null types, properties, required, additionalProperties,
// items, and enum. It is not a full JSON Schema (draft 2020-12)
// implementation — see the T1 plan's Global Constraints for what's out of
// scope and why.
package schema

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// Schema is a parsed JSON Schema (the subset this package supports).
type Schema struct {
	Type                 string             `json:"type,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	AdditionalProperties *bool              `json:"additionalProperties,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	Enum                 []any              `json:"enum,omitempty"`
}

// Parse decodes raw JSON Schema bytes into a Schema.
func Parse(raw json.RawMessage) (*Schema, error) {
	var s Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("schema: parse: %w", err)
	}
	return &s, nil
}

// maxValidationDepth bounds Validate's recursion into nested
// properties/items. Tool schemas are learned from an upstream server's own
// tools/list response — exactly the input a malicious or compromised
// upstream controls (the tool-poisoning threat model this product exists
// to defend against) — so unbounded recursion here is attacker-reachable,
// not just a theoretical concern.
const maxValidationDepth = 100

// Validate checks value (already JSON-decoded: map[string]any, []any,
// string, float64, bool, or nil) against s, returning a *ValidationError
// describing every violation found, or nil if value is valid.
func (s *Schema) Validate(value any) error {
	violations := s.validate(value, "$", 0)
	if len(violations) == 0 {
		return nil
	}
	return &ValidationError{Violations: violations}
}

func (s *Schema) validate(value any, path string, depth int) []string {
	if depth > maxValidationDepth {
		return []string{fmt.Sprintf("%s: exceeded maximum validation depth (%d)", path, maxValidationDepth)}
	}

	var violations []string

	if s.Type != "" && !typeMatches(s.Type, value) {
		return []string{fmt.Sprintf("%s: want type %q, got %s", path, s.Type, jsonTypeName(value))}
	}

	switch v := value.(type) {
	case map[string]any:
		for _, req := range s.Required {
			if _, present := v[req]; !present {
				violations = append(violations, fmt.Sprintf("%s: missing required property %q", path, req))
			}
		}
		for key, val := range v {
			if propSchema, ok := s.Properties[key]; ok {
				violations = append(violations, propSchema.validate(val, path+"."+key, depth+1)...)
			} else if s.AdditionalProperties != nil && !*s.AdditionalProperties {
				violations = append(violations, fmt.Sprintf("%s: additional property %q is not allowed", path, key))
			}
		}
	case []any:
		if s.Items != nil {
			for i, item := range v {
				violations = append(violations, s.Items.validate(item, fmt.Sprintf("%s[%d]", path, i), depth+1)...)
			}
		}
	}

	if len(s.Enum) > 0 && !enumContains(s.Enum, value) {
		violations = append(violations, fmt.Sprintf("%s: value not in enum", path))
	}

	return violations
}

func enumContains(enum []any, value any) bool {
	for _, e := range enum {
		if reflect.DeepEqual(e, value) {
			return true
		}
	}
	return false
}

func typeMatches(want string, value any) bool {
	switch want {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		f, ok := value.(float64)
		return ok && f == float64(int64(f))
	case "null":
		return value == nil
	default:
		return true // unknown declared type: don't block on it
	}
}

func jsonTypeName(value any) string {
	switch value.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", value)
	}
}

// ValidationError reports every schema violation found for one value.
type ValidationError struct {
	Violations []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("schema validation failed: %v", e.Violations)
}
