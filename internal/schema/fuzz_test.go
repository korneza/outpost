package schema

import (
	"encoding/json"
	"testing"
)

// FuzzValidate feeds arbitrary schema+value pairs at Parse and Validate.
// The schema half of this input is learned from an upstream server's own
// tools/list response — a malicious or compromised upstream fully controls
// it, which is exactly the tool-poisoning threat model this product exists
// to defend against. This must never panic.
func FuzzValidate(f *testing.F) {
	seeds := []struct {
		schemaJSON string
		valueJSON  string
	}{
		{`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`, `{"path":"x"}`},
		{`{"type":"array","items":{"type":"string"}}`, `["a","b"]`},
		{`{"type":"object","additionalProperties":false}`, `{"x":1}`},
		{`{"enum":["a","b"]}`, `"c"`},
		{`{"type":"integer"}`, `3.5`},
		{`not json`, `{}`},
		{`{}`, `not json`},
		{`{"properties":null}`, `{}`},
		{`{"type":123}`, `{}`},
		{`null`, `null`},
	}
	for _, s := range seeds {
		f.Add(s.schemaJSON, s.valueJSON)
	}
	f.Fuzz(func(_ *testing.T, schemaJSON, valueJSON string) {
		sch, err := Parse(json.RawMessage(schemaJSON))
		if err != nil {
			return // invalid schema JSON is expected to error, not panic
		}
		var value any
		if err := json.Unmarshal([]byte(valueJSON), &value); err != nil {
			return
		}
		_ = sch.Validate(value) // must never panic regardless of shape or depth
	})
}
