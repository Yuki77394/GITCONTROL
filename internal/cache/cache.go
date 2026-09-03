// Package cache provides a small generic in-memory TTL cache used across
// the bot for OAuth states, admin lists, message contexts, etc.
//
// The cache is intentionally minimal and not distributed. For multi-replica
// deployments, callers should rely on MongoDB-backed storage for state that
// must survive across instances (e.g. OAuth states are stored in DB too).
package cache

import (
	"sync"
	"time"
)

// Cache is a generic TTL cache safe for concurrent use.
type Cache[K comparable, V any] struct {
	items sync.Map
}

type entry[V any] struct {
	value      V
	expiration time.Time
}

// New creates a new Cache.
func New[K comparable, V any]() *Cache[K, V] {
	return &Cache[K, V]{}
}

// Set stores a value with the given TTL.
func (c *Cache[K, V]) Set(key K, value V, ttl time.Duration) {
	c.items.Store(key, entry[V]{value: value, expiration: time.Now().Add(ttl)})
}

// Get retrieves a value. Returns false if missing or expired.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	v, ok := c.items.Load(key)
	if !ok {
		var zero V
		return zero, false
	}
	e := v.(entry[V])
	if time.Now().After(e.expiration) {
		c.items.Delete(key)
		var zero V
		return zero, false
	}
	return e.value, true
}

// Delete removes an entry.
func (c *Cache[K, V]) Delete(key K) {
	c.items.Delete(key)
}

// Cleanup removes all expired entries. Intended to be called periodically.
func (c *Cache[K, V]) Cleanup() {
	c.items.Range(func(key, value any) bool {
		e := value.(entry[V])
		if time.Now().After(e.expiration) {
			c.items.Delete(key)
		}
		return true
	})
}

// Len returns the number of entries (including possibly expired ones).
// Mainly for tests / observability.
func (c *Cache[K, V]) Len() int {
	n := 0
	c.items.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}
