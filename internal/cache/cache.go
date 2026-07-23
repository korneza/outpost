// Package cache implements Outpost's list-op cache: a small, TTL-bounded
// in-memory cache for tools/list and resources/read responses. tools/call
// is uncacheable by construction (spec §2.2) — Key refuses to mint a cache
// key for it, so even a wiring mistake in the proxy degrades to a cache
// miss, never a stale/cached tool-call result.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/korneza/outpost/internal/boundedset"
	"github.com/korneza/outpost/internal/mcp"
)

// maxEntries bounds how many cache entries can exist at once. Key
// hashes attacker-controlled params with no size cap, and neither Set
// nor Get ever evicted an entry — Get only checked expiry on read, so
// an attacker sending many requests with distinct params could grow
// the map without bound, an effect that outlives the TTL since nothing
// ever swept it (Claude Security finding F11). Generous headroom for
// any real deployment's actual response cardinality, not a tight
// operational budget.
const maxEntries = 50_000

type entry struct {
	resp    json.RawMessage
	expires time.Time
}

// Cache is a TTL-bounded in-memory response cache. Zero value is not
// usable; construct with New.
type Cache struct {
	ttl     time.Duration
	mu      sync.Mutex
	m       map[string]entry
	tracked *boundedset.Tracker
}

// New returns a Cache whose entries expire ttl after being Set.
func New(ttl time.Duration) *Cache {
	return &Cache{ttl: ttl, m: make(map[string]entry), tracked: boundedset.New(maxEntries)}
}

var cacheableMethods = map[string]bool{
	mcp.MethodToolsList:     true,
	mcp.MethodResourcesRead: true,
}

// Key derives a cache key for method/upstream/params/authHeader. ok is
// false for any method other than tools/list or resources/read — most
// importantly tools/call, which must never be cached (spec §2.2).
//
// authHeader — the caller's forwarded Authorization header — is folded
// into the key alongside method/upstream/params. Outpost forwards each
// caller's bearer token to the upstream verbatim and never inspects it
// (internal/upstream/client.go), which means an upstream may legitimately
// return caller-scoped, authorization-gated content for the identical
// method+params. Without the caller's identity in the key, one caller's
// authorized response would be cached and then served straight back to
// any other caller — including one with no credentials at all — for the
// rest of the TTL, bypassing the upstream's own per-request authorization
// check entirely from within Outpost's trusted-proxy layer.
func (c *Cache) Key(method, upstream string, params json.RawMessage, authHeader string) (string, bool) {
	if !cacheableMethods[method] {
		return "", false
	}
	h := sha256.New()
	h.Write([]byte(method))
	h.Write([]byte{0})
	h.Write([]byte(upstream))
	h.Write([]byte{0})
	h.Write(params)
	h.Write([]byte{0})
	h.Write([]byte(authHeader))
	return hex.EncodeToString(h.Sum(nil)), true
}

// Get returns the cached response for key, if present and unexpired. An
// expired entry found here is actively deleted rather than left for Set
// to eventually evict — nothing else in this package sweeps on a timer.
func (c *Cache) Get(key string) (json.RawMessage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expires) {
		delete(c.m, key)
		return nil, false
	}
	return e.resp, true
}

// Set stores resp under key with the cache's configured TTL, evicting
// the oldest entry if this insertion pushes the cache over maxEntries.
func (c *Cache) Set(key string, resp json.RawMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = entry{resp: resp, expires: time.Now().Add(c.ttl)}
	if evict, evicted := c.tracked.Add(key); evicted {
		delete(c.m, evict)
	}
}
