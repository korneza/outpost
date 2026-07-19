package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type goldenCase struct {
	Schema    json.RawMessage `json:"schema"`
	Arguments json.RawMessage `json:"arguments"`
	Valid     bool            `json:"valid"`
}

func TestGoldenCorpus(t *testing.T) {
	files, err := filepath.Glob("testdata/golden/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no golden corpus fixtures found under testdata/golden")
	}
	for _, f := range files {
		f := f
		t.Run(filepath.Base(f), func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var c goldenCase
			if err := json.Unmarshal(data, &c); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			sch, err := Parse(c.Schema)
			if err != nil {
				t.Fatalf("parse schema: %v", err)
			}
			var args any
			if err := json.Unmarshal(c.Arguments, &args); err != nil {
				t.Fatalf("decode arguments: %v", err)
			}
			gotValid := sch.Validate(args) == nil
			if gotValid != c.Valid {
				t.Fatalf("Validate() valid=%v, want %v", gotValid, c.Valid)
			}
		})
	}
}
