package cache

import (
	"testing"
	"time"

	"github.com/korneza/outpost/internal/mcp"
)

func TestKeyRejectsToolsCallByConstruction(t *testing.T) {
	c := New(time.Minute)
	_, ok := c.Key(mcp.MethodToolsCall, "up1", []byte(`{"name":"x"}`), "")
	if ok {
		t.Fatal("Key must never mint a cache key for tools/call")
	}
}

func TestKeyAcceptsToolsListAndResourcesRead(t *testing.T) {
	c := New(time.Minute)
	for _, method := range []string{mcp.MethodToolsList, mcp.MethodResourcesRead} {
		if _, ok := c.Key(method, "up1", nil, ""); !ok {
			t.Fatalf("Key(%s, ...) should mint a key", method)
		}
	}
}

func TestSetThenGetReturnsStoredResponse(t *testing.T) {
	c := New(time.Minute)
	key, _ := c.Key(mcp.MethodToolsList, "up1", nil, "")
	c.Set(key, []byte(`{"tools":[]}`))
	got, ok := c.Get(key)
	if !ok || string(got) != `{"tools":[]}` {
		t.Fatalf("Get returned (%s, %v), want ({\"tools\":[]}, true)", got, ok)
	}
}

func TestGetExpiresAfterTTL(t *testing.T) {
	c := New(10 * time.Millisecond)
	key, _ := c.Key(mcp.MethodToolsList, "up1", nil, "")
	c.Set(key, []byte(`{}`))
	time.Sleep(20 * time.Millisecond)
	if _, ok := c.Get(key); ok {
		t.Fatal("entry should have expired")
	}
}

func TestKeyDiffersByUpstreamAndParams(t *testing.T) {
	c := New(time.Minute)
	k1, _ := c.Key(mcp.MethodResourcesRead, "up1", []byte(`{"uri":"a"}`), "")
	k2, _ := c.Key(mcp.MethodResourcesRead, "up1", []byte(`{"uri":"b"}`), "")
	k3, _ := c.Key(mcp.MethodResourcesRead, "up2", []byte(`{"uri":"a"}`), "")
	if k1 == k2 || k1 == k3 {
		t.Fatal("keys must differ by params and upstream")
	}
}

// TestKeyDiffersByCallerIdentity is the fix for the cache cross-caller
// leak (Claude Security findings F2/F12/F13/F15): two callers presenting
// different Authorization headers for the identical method/upstream/
// params must never collide on the same cache key, or a caller-scoped
// (authorization-gated) response from one caller would be served
// straight back to a different, unauthorized caller within the TTL.
func TestKeyDiffersByCallerIdentity(t *testing.T) {
	c := New(time.Minute)
	kA, _ := c.Key(mcp.MethodResourcesRead, "up1", []byte(`{"uri":"a"}`), "Bearer token-a")
	kB, _ := c.Key(mcp.MethodResourcesRead, "up1", []byte(`{"uri":"a"}`), "Bearer token-b")
	kNone, _ := c.Key(mcp.MethodResourcesRead, "up1", []byte(`{"uri":"a"}`), "")
	if kA == kB {
		t.Fatal("two different Authorization headers must not produce the same cache key")
	}
	if kA == kNone || kB == kNone {
		t.Fatal("a caller with credentials must not collide with a caller presenting none")
	}
}

// TestKeySameForRepeatedCallerIdentity confirms the fix doesn't break
// the cache's actual purpose: the SAME caller (same Authorization
// header) making the same request repeatedly must still hit the cache.
func TestKeySameForRepeatedCallerIdentity(t *testing.T) {
	c := New(time.Minute)
	k1, _ := c.Key(mcp.MethodResourcesRead, "up1", []byte(`{"uri":"a"}`), "Bearer token-a")
	k2, _ := c.Key(mcp.MethodResourcesRead, "up1", []byte(`{"uri":"a"}`), "Bearer token-a")
	if k1 != k2 {
		t.Fatal("identical caller + method + upstream + params must still produce the same key")
	}
}

// TestEntryCountIsBounded guards against Claude Security finding F11:
// Key hashes attacker-controlled params with no size cap, Set never
// evicts, and Get only checks expiry on read rather than actively
// purging — so an attacker sending many resources/read requests with
// distinct params could grow the cache's entry count without bound,
// an effect that outlives the TTL since nothing ever sweeps it.
func TestEntryCountIsBounded(t *testing.T) {
	c := New(time.Minute)
	for i := 0; i < maxEntries+500; i++ {
		key, _ := c.Key(mcp.MethodResourcesRead, "up1", []byte(string(rune(i))), "")
		c.Set(key, []byte(`{}`))
	}
	c.mu.Lock()
	n := len(c.m)
	c.mu.Unlock()
	if n > maxEntries {
		t.Fatalf("entry count = %d, want capped at %d", n, maxEntries)
	}
}
