package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestLogCallSuccessHasOnlyAllowedFields(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)
	LogCall(logger, "files", "tools/call", "files.read", 12*time.Millisecond, nil)

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log output is not valid JSON: %v\n%s", err, buf.String())
	}
	allowed := map[string]bool{
		"time": true, "level": true, "msg": true,
		"upstream": true, "method": true, "tool": true, "duration": true,
	}
	for k := range line {
		if !allowed[k] {
			t.Errorf("log line has unexpected field %q — only metadata fields are allowed, never payload data: %s", k, buf.String())
		}
	}
	if line["tool"] != "files.read" {
		t.Errorf("tool = %v, want files.read", line["tool"])
	}
}

func TestLogCallErrorIncludesErrorField(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf)
	LogCall(logger, "files", "tools/call", "files.read", time.Millisecond, errors.New("connection refused"))

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log output is not valid JSON: %v", err)
	}
	if line["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", line["level"])
	}
	if line["error"] != "connection refused" {
		t.Errorf("error = %v, want %q", line["error"], "connection refused")
	}
}
