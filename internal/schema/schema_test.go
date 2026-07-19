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
