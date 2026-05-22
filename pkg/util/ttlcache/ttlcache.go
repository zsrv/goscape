// Package ttlcache provides a minimal time-based TTL cache used by the
// world login rate-limiter. Entries expire after a fixed TTL set at
// construction time. Expiry is lazy: a stale entry is dropped on the next
// Get/Set/Increment call for that key, not on a background sweeper.
//
// Mirrors the JS `@isaacs/ttlcache` usage in
// LostCityRS/Engine-TS/src/engine/World.ts:176-177 where
// `new TTLCache({ ttl: 60000 })` is used for login attempt tracking.
package ttlcache

import (
	"sync"
	"time"
)

type entry[V any] struct {
	value     V
	expiresAt time.Time
}

// Cache is a goroutine-safe key/value store with a single fixed TTL.
// The zero value is not usable — construct with New.
type Cache[K comparable, V any] struct {
	ttl   time.Duration
	now   func() time.Time
	mu    sync.Mutex
	items map[K]entry[V]
}

// New constructs a Cache with the given fixed entry TTL. ttl must be > 0.
func New[K comparable, V any](ttl time.Duration) *Cache[K, V] {
	return &Cache[K, V]{
		ttl:   ttl,
		now:   time.Now,
		items: make(map[K]entry[V]),
	}
}

// setNowFunc swaps the time source. Test-only seam.
func (c *Cache[K, V]) setNowFunc(now func() time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

// Get returns the value stored under key and true if present and unexpired.
// Expired entries are evicted lazily.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var zero V
	e, ok := c.items[key]
	if !ok {
		return zero, false
	}
	if !c.now().Before(e.expiresAt) {
		delete(c.items, key)
		return zero, false
	}
	return e.value, true
}

// Set stores value at key with a fresh TTL window starting now.
func (c *Cache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = entry[V]{value: value, expiresAt: c.now().Add(c.ttl)}
}

// Len returns the number of stored entries, including ones that may be
// past their TTL but not yet evicted. Intended for tests and metrics.
func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// Delete removes key if present.
func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}
