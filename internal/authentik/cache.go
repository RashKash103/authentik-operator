package authentik

import (
	"sync"
	"time"
)

// DefaultCacheTTL is the lifetime of a cached reference lookup. It is short on
// purpose: a stale UUID is worse than an extra API call, and reconciles are
// bursty enough that even a few seconds removes most duplicate lookups.
const DefaultCacheTTL = 30 * time.Second

// RefKind identifies the type of object a cached reference points at. It is
// part of the cache key so a flow and a property mapping with the same name
// never collide.
type RefKind string

// The reference kinds the resolvers understand.
const (
	// KindFlow is a flow, addressed by slug and resolved to a UUID.
	KindFlow RefKind = "flow"
	// KindPropertyMapping is a property mapping, addressed by name and resolved to a UUID.
	KindPropertyMapping RefKind = "property-mapping"
	// KindCertificateKeyPair is a certificate-key pair, addressed by name and resolved to a UUID.
	KindCertificateKeyPair RefKind = "certificate-keypair"
	// KindProvider is a provider, addressed by name and resolved to an integer primary key.
	KindProvider RefKind = "provider"
	// KindServiceConnection is an outpost service connection, addressed by name and resolved to a UUID.
	KindServiceConnection RefKind = "service-connection"
)

// String returns the kind as a plain string, for use in messages.
func (k RefKind) String() string { return string(k) }

// cacheKey identifies one cached lookup. The connection component is what makes
// the cache safe to share: two authentik instances routinely have a flow with
// the same slug that means completely different things.
type cacheKey struct {
	conn string
	kind RefKind
	name string
}

type cacheEntry struct {
	value     string
	expiresAt time.Time
}

// RefCache is a concurrency-safe, short-TTL cache of resolved references keyed
// by (connection, kind, name).
//
// Only successful resolutions are stored. A lookup that misses removes any
// entry it finds for that key, so an object created in authentik after a failed
// reconcile is picked up on the next attempt instead of waiting out a TTL.
//
// The zero value is not usable; call NewRefCache.
type RefCache struct {
	mu      sync.RWMutex
	ttl     time.Duration
	entries map[cacheKey]cacheEntry

	// now is swapped out in tests to exercise expiry without sleeping.
	now func() time.Time
}

// NewRefCache returns an empty cache whose entries live for ttl. A ttl of zero
// or less selects DefaultCacheTTL.
func NewRefCache(ttl time.Duration) *RefCache {
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	return &RefCache{
		ttl:     ttl,
		entries: make(map[cacheKey]cacheEntry),
		now:     time.Now,
	}
}

// TTL returns the lifetime applied to newly stored entries.
func (c *RefCache) TTL() time.Duration { return c.ttl }

// Get returns the cached value for (conn, kind, name). An entry past its TTL is
// treated as absent and removed.
func (c *RefCache) Get(conn string, kind RefKind, name string) (string, bool) {
	key := cacheKey{conn: conn, kind: kind, name: name}

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	if !c.now().Before(entry.expiresAt) {
		c.mu.Lock()
		// Re-check: another goroutine may have refreshed the entry meanwhile.
		if current, still := c.entries[key]; still && current.expiresAt.Equal(entry.expiresAt) {
			delete(c.entries, key)
		}
		c.mu.Unlock()
		return "", false
	}
	return entry.value, true
}

// Set stores value for (conn, kind, name) for one TTL. An empty value is
// ignored: negative results are never cached.
func (c *RefCache) Set(conn string, kind RefKind, name, value string) {
	if value == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[cacheKey{conn: conn, kind: kind, name: name}] = cacheEntry{
		value:     value,
		expiresAt: c.now().Add(c.ttl),
	}
}

// Invalidate drops the entry for (conn, kind, name), if any.
func (c *RefCache) Invalidate(conn string, kind RefKind, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, cacheKey{conn: conn, kind: kind, name: name})
}

// InvalidateConnection drops every entry belonging to one authentik instance.
// Use it when a connection's credentials or CA bundle change.
func (c *RefCache) InvalidateConnection(conn string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.entries {
		if key.conn == conn {
			delete(c.entries, key)
		}
	}
}

// Purge empties the cache.
func (c *RefCache) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[cacheKey]cacheEntry)
}

// Len returns the number of entries held, including any that have expired but
// have not yet been reclaimed. It exists for tests and metrics.
func (c *RefCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}
