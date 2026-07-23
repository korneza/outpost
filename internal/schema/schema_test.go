package schema

import (
	"encoding/json"
	"testing"
)

func mustParse(t *testing.T, raw string) *Schema {
	t.Helper()
	s, err := Parse(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return s
}

func decode(t *testing.T, raw string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

func TestValidateAcceptsMatchingObject(t *testing.T) {
	s := mustParse(t, `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)
	if err := s.Validate(decode(t, `{"path":"/tmp/x"}`)); err != nil {
		t.Fatalf("Validate: %v, want nil", err)
	}
}

func TestValidateRejectsMissingRequiredProperty(t *testing.T) {
	s := mustParse(t, `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)
	err := s.Validate(decode(t, `{}`))
	if err == nil {
		t.Fatal("Validate: want an error for missing required property, got nil")
	}
}

func TestValidateRejectsWrongScalarType(t *testing.T) {
	s := mustParse(t, `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)
	err := s.Validate(decode(t, `{"path":123}`))
	if err == nil {
		t.Fatal("Validate: want an error for path being a number, want string, got nil")
	}
}

func TestValidateRejectsWrongTopLevelType(t *testing.T) {
	s := mustParse(t, `{"type":"object"}`)
	err := s.Validate(decode(t, `"not an object"`))
	if err == nil {
		t.Fatal("Validate: want an error for a string where an object was required, got nil")
	}
}

func TestValidateAcceptsAllScalarTypes(t *testing.T) {
	cases := []struct {
		schemaType string
		value      string
	}{
		{"string", `"hello"`},
		{"number", `3.14`},
		{"integer", `42`},
		{"boolean", `true`},
		{"null", `null`},
	}
	for _, c := range cases {
		s := mustParse(t, `{"type":"`+c.schemaType+`"}`)
		if err := s.Validate(decode(t, c.value)); err != nil {
			t.Errorf("type %s: Validate(%s) = %v, want nil", c.schemaType, c.value, err)
		}
	}
}

func TestValidateRejectsIntegerWithFraction(t *testing.T) {
	s := mustParse(t, `{"type":"integer"}`)
	if err := s.Validate(decode(t, `3.5`)); err == nil {
		t.Fatal("Validate: want an error for 3.5 against type integer, got nil")
	}
}

func TestValidateRecursesIntoNestedProperty(t *testing.T) {
	s := mustParse(t, `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)
	err := s.Validate(decode(t, `{"path":123}`))
	if err == nil {
		t.Fatal("Validate: want an error, the nested path property is the wrong type")
	}
}

func TestValidateRejectsAdditionalPropertyWhenDisallowed(t *testing.T) {
	no := false
	s := &Schema{
		Type:                 "object",
		Properties:           map[string]*Schema{"path": {Type: "string"}},
		Required:             []string{"path"},
		AdditionalProperties: &no,
	}
	err := s.Validate(decode(t, `{"path":"/tmp/x","evil_extra_field":"payload"}`))
	if err == nil {
		t.Fatal("Validate: want an error for a disallowed additional property, got nil")
	}
}

func TestValidateAllowsAdditionalPropertyWhenPermitted(t *testing.T) {
	s := mustParse(t, `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)
	// No additionalProperties set at all: nil means "not restricted".
	if err := s.Validate(decode(t, `{"path":"/tmp/x","extra":"fine"}`)); err != nil {
		t.Fatalf("Validate: %v, want nil — additionalProperties unset means unrestricted", err)
	}
}

func TestValidateChecksArrayItems(t *testing.T) {
	s := mustParse(t, `{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}}`)
	if err := s.Validate(decode(t, `[{"id":"a"},{"id":"b"}]`)); err != nil {
		t.Fatalf("Validate valid array: %v, want nil", err)
	}
	err := s.Validate(decode(t, `[{"id":"a"},{}]`))
	if err == nil {
		t.Fatal("Validate: want an error, second array item is missing required id")
	}
}

func TestValidateChecksEnum(t *testing.T) {
	s := mustParse(t, `{"type":"string","enum":["read","write"]}`)
	if err := s.Validate(decode(t, `"read"`)); err != nil {
		t.Fatalf("Validate: %v, want nil for an enum member", err)
	}
	if err := s.Validate(decode(t, `"delete"`)); err == nil {
		t.Fatal("Validate: want an error for a non-enum value, got nil")
	}
}

// TestValidateDoesNotPanicOnNullPropertySchema guards against Claude
// Security finding F8: a literal JSON null for a property in an
// upstream-controlled inputSchema unmarshals to a nil *Schema with no
// error. The old code's `if propSchema, ok := s.Properties[key]; ok`
// only checked map-key presence, not pointer nilness, so it recursed
// into the nil *Schema and panicked dereferencing its Type field —
// crashing the goroutine handling the request (and, in stdio mode, the
// whole process — see cmd/outpost's run command, no recover() anywhere
// in this repo). This must degrade to "no constraint enforced for that
// key," never a crash.
func TestValidateDoesNotPanicOnNullPropertySchema(t *testing.T) {
	s := mustParse(t, `{"type":"object","properties":{"path":null},"required":[]}`)
	// Must not panic. Whatever verdict it reaches for the property with
	// a null schema is secondary to not crashing the process.
	_ = s.Validate(decode(t, `{"path":"anything"}`))
}

// TestValidateEnforcesObjectShapeWhenImpliedByPropertiesEvenWithoutType
// guards against Claude Security finding F19: a schema that declares
// required/properties/additionalProperties but omits an explicit
// top-level "type":"object" previously skipped all three checks
// entirely if the actual value wasn't already a Go map — required-field
// and additional-property enforcement lived only inside the
// map[string]any case of a switch keyed on the value's *runtime* type,
// never the schema's declared shape. A non-object value (e.g. a bare
// string) sailed through as "valid" even though the schema clearly
// implies an object.
func TestValidateEnforcesObjectShapeWhenImpliedByPropertiesEvenWithoutType(t *testing.T) {
	s := mustParse(t, `{"properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`)
	if err := s.Validate(decode(t, `"bypass"`)); err == nil {
		t.Fatal("Validate: want an error — a bare string must not satisfy a schema that declares required/properties/additionalProperties")
	}
	// The legitimate object case must still work exactly as before.
	if err := s.Validate(decode(t, `{"path":"/tmp/x"}`)); err != nil {
		t.Fatalf("Validate: %v, want nil for a genuinely matching object", err)
	}
	if err := s.Validate(decode(t, `{}`)); err == nil {
		t.Fatal("Validate: want an error for a missing required property, even without an explicit type")
	}
}

// TestTypeMatchesRejectsUnrecognizedTypeString guards against the other
// half of F19: typeMatches's default case returned true for ANY
// unrecognized type string, meaning a typo'd or attacker-crafted type
// (anything other than the 7 standard JSON Schema primitives, all of
// which are already handled above this default) silently matched every
// value regardless of shape.
func TestTypeMatchesRejectsUnrecognizedTypeString(t *testing.T) {
	if typeMatches("not-a-real-type", "anything") {
		t.Fatal("typeMatches: want false for an unrecognized type string, got true")
	}
}
