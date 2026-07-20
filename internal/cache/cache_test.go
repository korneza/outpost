package cache

import (
	"testing"
	"time"

	"github.com/korneza/outpost/internal/mcp"
)

func TestKeyRejectsToolsCallByConstruction(t *testing.T) {
	c := New(time.Minute)
	_, ok := c.Key(mcp.MethodToolsCall, "up1", []byte(`{"name":"x"}`))
	if ok {
		t.Fatal("Key must never mint a cache key for tools/call")
	}
}

func TestKeyAcceptsToolsListAndResourcesRead(t *testing.T) {
	c := New(time.Minute)
	for _, method := range []string{mcp.MethodToolsList, mcp.MethodResourcesRead} {
		if _, ok := c.Key(method, "up1", nil); !ok {
			t.Fatalf("Key(%s, ...) should mint a key", method)
		}
	}
}

func TestSetThenGetReturnsStoredResponse(t *testing.T) {
	c := New(time.Minute)
	key, _ := c.Key(mcp.MethodToolsList, "up1", nil)
	c.Set(key, []byte(`{"tools":[]}`))
	got, ok := c.Get(key)
	if !ok || string(got) != `{"tools":[]}` {
		t.Fatalf("Get returned (%s, %v), want ({\"tools\":[]}, true)", got, ok)
	}
}

func TestGetExpiresAfterTTL(t *testing.T) {
	c := New(10 * time.Millisecond)
	key, _ := c.Key(mcp.MethodToolsList, "up1", nil)
	c.Set(key, []byte(`{}`))
	time.Sleep(20 * time.Millisecond)
	if _, ok := c.Get(key); ok {
		t.Fatal("entry should have expired")
	}
}

func TestKeyDiffersByUpstreamAndParams(t *testing.T) {
	c := New(time.Minute)
	k1, _ := c.Key(mcp.MethodResourcesRead, "up1", []byte(`{"uri":"a"}`))
	k2, _ := c.Key(mcp.MethodResourcesRead, "up1", []byte(`{"uri":"b"}`))
	k3, _ := c.Key(mcp.MethodResourcesRead, "up2", []byte(`{"uri":"a"}`))
	if k1 == k2 || k1 == k3 {
		t.Fatal("keys must differ by params and upstream")
	}
}
