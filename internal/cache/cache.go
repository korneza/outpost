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

	"github.com/korneza/outpost/internal/mcp"
)

type entry struct {
	resp    json.RawMessage
	expires time.Time
}

// Cache is a TTL-bounded in-memory response cache. Zero value is not
// usable; construct with New.
type Cache struct {
	ttl time.Duration
	mu  sync.Mutex
	m   map[string]entry
}

// New returns a Cache whose entries expire ttl after being Set.
func New(ttl time.Duration) *Cache {
	return &Cache{ttl: ttl, m: make(map[string]entry)}
}

var cacheableMethods = map[string]bool{
	mcp.MethodToolsList:     true,
	mcp.MethodResourcesRead: true,
}

// Key derives a cache key for method/upstream/params. ok is false for any
// method other than tools/list or resources/read — most importantly
// tools/call, which must never be cached (spec §2.2).
func (c *Cache) Key(method, upstream string, params json.RawMessage) (string, bool) {
	if !cacheableMethods[method] {
		return "", false
	}
	h := sha256.New()
	h.Write([]byte(method))
	h.Write([]byte{0})
	h.Write([]byte(upstream))
	h.Write([]byte{0})
	h.Write(params)
	return hex.EncodeToString(h.Sum(nil)), true
}

// Get returns the cached response for key, if present and unexpired.
func (c *Cache) Get(key string) (json.RawMessage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.resp, true
}

// Set stores resp under key with the cache's configured TTL.
func (c *Cache) Set(key string, resp json.RawMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = entry{resp: resp, expires: time.Now().Add(c.ttl)}
}
