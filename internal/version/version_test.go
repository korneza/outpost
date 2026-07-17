package version

import (
	"strings"
	"testing"
)

func TestStringDefaultsToDev(t *testing.T) {
	s := String()
	if !strings.HasPrefix(s, "outpost dev") {
		t.Fatalf("String() = %q, want prefix %q", s, "outpost dev")
	}
	if !strings.Contains(s, "commit none") || !strings.Contains(s, "built unknown") {
		t.Fatalf("String() = %q, want default commit/date markers", s)
	}
}
