// Package boundedset provides a tiny capacity-bounded insertion tracker
// for packages that key an unbounded in-memory map on attacker-influenced
// strings (a tool name, a cache key). It tracks insertion order only —
// callers keep their own map and their own synchronization; Tracker adds
// no locking of its own, so it's meant to be used from inside a lock the
// caller already holds.
//
// This is FIFO eviction, not LRU: the oldest-inserted key is evicted on
// overflow regardless of how recently it was read. That's a deliberate
// simplification — the security property that actually matters here is
// "the map cannot grow without bound," not recency-optimal eviction, and
// FIFO gets that with a single slice and no extra locking to reason
// about alongside each caller's existing mutex.
package boundedset

// Tracker records insertion order for up to capacity keys.
type Tracker struct {
	capacity int
	order    []string
	present  map[string]bool
}

// New returns a Tracker bounded to capacity keys. capacity < 1 is
// treated as 1.
func New(capacity int) *Tracker {
	if capacity < 1 {
		capacity = 1
	}
	return &Tracker{capacity: capacity, present: make(map[string]bool, capacity)}
}

// Add records key as a newly-inserted entry. Call this only when the
// caller is about to insert a genuinely new key into its own map — not
// on an update to a key already present, which Add would otherwise
// (harmlessly, but pointlessly) re-track.
//
// Returns the key to evict from the caller's map, if recording this
// insertion pushed the tracker over capacity (ok is true in that case).
// A caller ignoring a false ok is exactly the no-op case: nothing to
// evict yet.
func (t *Tracker) Add(key string) (evict string, ok bool) {
	if t.present[key] {
		return "", false
	}
	t.present[key] = true
	t.order = append(t.order, key)
	if len(t.order) <= t.capacity {
		return "", false
	}
	evict = t.order[0]
	t.order = t.order[1:]
	delete(t.present, evict)
	return evict, true
}

// Len reports how many keys the tracker currently holds.
func (t *Tracker) Len() int {
	return len(t.order)
}
