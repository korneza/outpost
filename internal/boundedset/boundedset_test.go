package boundedset

import "testing"

func TestAddBelowCapacityNeverEvicts(t *testing.T) {
	tr := New(3)
	for _, k := range []string{"a", "b", "c"} {
		if _, ok := tr.Add(k); ok {
			t.Fatalf("Add(%q) evicted below capacity", k)
		}
	}
	if tr.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", tr.Len())
	}
}

func TestAddOverCapacityEvictsOldestFirst(t *testing.T) {
	tr := New(2)
	tr.Add("a")
	tr.Add("b")
	evict, ok := tr.Add("c")
	if !ok || evict != "a" {
		t.Fatalf("Add(c) = (%q, %v), want (\"a\", true) — oldest key evicted", evict, ok)
	}
	if tr.Len() != 2 {
		t.Fatalf("Len() = %d, want 2 (bounded)", tr.Len())
	}
	evict, ok = tr.Add("d")
	if !ok || evict != "b" {
		t.Fatalf("Add(d) = (%q, %v), want (\"b\", true)", evict, ok)
	}
}

func TestAddDuplicateKeyIsNoOp(t *testing.T) {
	tr := New(2)
	tr.Add("a")
	tr.Add("b")
	if evict, ok := tr.Add("a"); ok {
		t.Fatalf("Add on an already-present key evicted (%q), want no-op", evict)
	}
	if tr.Len() != 2 {
		t.Fatalf("Len() = %d, want 2 unchanged", tr.Len())
	}
}

func TestNewClampsCapacityBelowOne(t *testing.T) {
	tr := New(0)
	if _, ok := tr.Add("a"); ok {
		t.Fatal("first Add on a clamped-to-1 tracker should not evict")
	}
	if evict, ok := tr.Add("b"); !ok || evict != "a" {
		t.Fatalf("Add(b) = (%q, %v), want (\"a\", true) — capacity clamped to 1", evict, ok)
	}
}

func TestUnboundedGrowthCannotExceedCapacity(t *testing.T) {
	// The actual security property under test: no matter how many
	// distinct keys are inserted, the tracked count never exceeds
	// capacity — this is what stands between an attacker sending
	// millions of fabricated names and unbounded memory growth.
	tr := New(100)
	for i := 0; i < 100_000; i++ {
		tr.Add(string(rune(i)))
	}
	if tr.Len() != 100 {
		t.Fatalf("Len() = %d after 100,000 inserts, want 100 (capacity)", tr.Len())
	}
}
