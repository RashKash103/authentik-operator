package authentik

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// newTestCache returns a cache whose clock is driven by the returned advance
// function, so expiry can be tested without sleeping.
func newTestCache(t *testing.T, ttl time.Duration) (*RefCache, func(time.Duration)) {
	t.Helper()

	var mu sync.Mutex
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	cache := NewRefCache(ttl)
	cache.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	return cache, func(d time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		now = now.Add(d)
	}
}

func TestRefCacheHitAndExpiry(t *testing.T) {
	t.Parallel()

	const ttl = 30 * time.Second
	cache, advance := newTestCache(t, ttl)

	cache.Set("conn-a", KindFlow, "default-authentication-flow", "uuid-1")

	if got, ok := cache.Get("conn-a", KindFlow, "default-authentication-flow"); !ok || got != "uuid-1" {
		t.Fatalf("Get = (%q, %v), want (uuid-1, true)", got, ok)
	}

	// Just inside the TTL the entry is still live.
	advance(ttl - time.Millisecond)
	if _, ok := cache.Get("conn-a", KindFlow, "default-authentication-flow"); !ok {
		t.Fatal("entry expired before its TTL elapsed")
	}

	// Exactly at the TTL it is gone, and reclaimed.
	advance(time.Millisecond)
	if _, ok := cache.Get("conn-a", KindFlow, "default-authentication-flow"); ok {
		t.Fatal("entry survived past its TTL")
	}
	if cache.Len() != 0 {
		t.Errorf("Len() = %d after expiry, want 0", cache.Len())
	}
}

func TestRefCacheKindsDoNotCollide(t *testing.T) {
	t.Parallel()

	cache, _ := newTestCache(t, time.Minute)
	cache.Set("conn-a", KindFlow, "shared-name", "flow-uuid")
	cache.Set("conn-a", KindPropertyMapping, "shared-name", "mapping-uuid")

	if got, _ := cache.Get("conn-a", KindFlow, "shared-name"); got != "flow-uuid" {
		t.Errorf("flow lookup = %q, want flow-uuid", got)
	}
	if got, _ := cache.Get("conn-a", KindPropertyMapping, "shared-name"); got != "mapping-uuid" {
		t.Errorf("property mapping lookup = %q, want mapping-uuid", got)
	}
}

// TestRefCacheIsPerConnection is the property that makes a shared cache safe:
// the same flow slug on two authentik instances means two different objects.
func TestRefCacheIsPerConnection(t *testing.T) {
	t.Parallel()

	cache, _ := newTestCache(t, time.Minute)
	cache.Set("https://a.example#aaaa", KindFlow, "default-source-enrollment", "uuid-from-a")
	cache.Set("https://b.example#bbbb", KindFlow, "default-source-enrollment", "uuid-from-b")

	if got, _ := cache.Get("https://a.example#aaaa", KindFlow, "default-source-enrollment"); got != "uuid-from-a" {
		t.Errorf("connection a resolved to %q, want uuid-from-a", got)
	}
	if got, _ := cache.Get("https://b.example#bbbb", KindFlow, "default-source-enrollment"); got != "uuid-from-b" {
		t.Errorf("connection b resolved to %q, want uuid-from-b", got)
	}
	if _, ok := cache.Get("https://c.example#cccc", KindFlow, "default-source-enrollment"); ok {
		t.Error("an unrelated connection read another connection's entry")
	}
}

func TestRefCacheInvalidate(t *testing.T) {
	t.Parallel()

	cache, _ := newTestCache(t, time.Minute)
	cache.Set("conn-a", KindFlow, "a", "1")
	cache.Set("conn-a", KindFlow, "b", "2")

	cache.Invalidate("conn-a", KindFlow, "a")
	if _, ok := cache.Get("conn-a", KindFlow, "a"); ok {
		t.Error("invalidated entry was still readable")
	}
	if _, ok := cache.Get("conn-a", KindFlow, "b"); !ok {
		t.Error("Invalidate removed an unrelated entry")
	}

	// Invalidating an absent key is a no-op, not a panic.
	cache.Invalidate("conn-a", KindFlow, "does-not-exist")
}

func TestRefCacheInvalidateConnectionAndPurge(t *testing.T) {
	t.Parallel()

	cache, _ := newTestCache(t, time.Minute)
	cache.Set("conn-a", KindFlow, "a", "1")
	cache.Set("conn-a", KindProvider, "p", "7")
	cache.Set("conn-b", KindFlow, "a", "2")

	cache.InvalidateConnection("conn-a")
	if _, ok := cache.Get("conn-a", KindFlow, "a"); ok {
		t.Error("connection invalidation left a flow entry behind")
	}
	if _, ok := cache.Get("conn-a", KindProvider, "p"); ok {
		t.Error("connection invalidation left a provider entry behind")
	}
	if _, ok := cache.Get("conn-b", KindFlow, "a"); !ok {
		t.Error("connection invalidation removed another connection's entry")
	}

	cache.Purge()
	if cache.Len() != 0 {
		t.Errorf("Len() = %d after Purge, want 0", cache.Len())
	}
}

func TestRefCacheDoesNotStoreEmptyValues(t *testing.T) {
	t.Parallel()

	cache, _ := newTestCache(t, time.Minute)
	cache.Set("conn-a", KindFlow, "missing", "")
	if _, ok := cache.Get("conn-a", KindFlow, "missing"); ok {
		t.Error("an empty value was cached; negative results must not be cached")
	}
}

func TestNewRefCacheDefaultsTTL(t *testing.T) {
	t.Parallel()

	for _, ttl := range []time.Duration{0, -time.Second} {
		if got := NewRefCache(ttl).TTL(); got != DefaultCacheTTL {
			t.Errorf("NewRefCache(%v).TTL() = %v, want %v", ttl, got, DefaultCacheTTL)
		}
	}
	if got := NewRefCache(5 * time.Second).TTL(); got != 5*time.Second {
		t.Errorf("TTL() = %v, want 5s", got)
	}
}

// TestRefCacheConcurrentAccess is meaningful under -race.
func TestRefCacheConcurrentAccess(t *testing.T) {
	t.Parallel()

	cache := NewRefCache(50 * time.Millisecond)

	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			conn := fmt.Sprintf("conn-%d", worker%3)
			for i := range 200 {
				name := fmt.Sprintf("name-%d", i%10)
				cache.Set(conn, KindFlow, name, fmt.Sprintf("uuid-%d", i))
				cache.Get(conn, KindFlow, name)
				if i%17 == 0 {
					cache.Invalidate(conn, KindFlow, name)
				}
				if i%97 == 0 {
					cache.InvalidateConnection(conn)
				}
				_ = cache.Len()
			}
		}(worker)
	}
	wg.Wait()
}
