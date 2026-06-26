// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"sync"
	"time"
)

// cacheKey identifies a cached verification key by subject and key id.
type cacheKey struct {
	subject string
	kid     string
}

// cacheEntry is a cached verification key with its absolute expiry.
type cacheEntry struct {
	key     VerificationKey
	expires time.Time
}

// keyCache caches resolved verification keys by (subject, kid) with a TTL so
// the verifier avoids a key-provider lookup on every request. It is safe for
// concurrent use. Expired entries are evicted lazily on read; a hard maxEntries
// cap bounds memory so a churn of short-lived subjects (one-off principals,
// rotated-away kids) cannot grow the map without limit.
type keyCache struct {
	ttl        time.Duration
	maxEntries int
	clock      Clock
	mu         sync.RWMutex
	items      map[cacheKey]cacheEntry
}

// newKeyCache builds an empty key cache with the given TTL, size cap, and clock.
func newKeyCache(ttl time.Duration, maxEntries int, clock Clock) *keyCache {
	return &keyCache{ttl: ttl, maxEntries: maxEntries, clock: clock, items: make(map[cacheKey]cacheEntry)}
}

// get returns the cached key for (subject, kid) when present and unexpired.
func (c *keyCache) get(subject, kid string) (VerificationKey, bool) {
	c.mu.RLock()
	e, ok := c.items[cacheKey{subject: subject, kid: kid}]
	c.mu.RUnlock()
	if !ok || !c.clock.Now().Before(e.expires) {
		return VerificationKey{}, false
	}
	return e.key, true
}

// put stores key for (subject, kid), expiring one TTL from now. When the cache
// is at its size cap it first drops expired entries and, if that frees nothing,
// evicts arbitrary entries until a slot is free, keeping the map bounded.
func (c *keyCache) put(subject, kid string, key VerificationKey) {
	now := c.clock.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.maxEntries > 0 && len(c.items) >= c.maxEntries {
		c.evictLocked(now)
	}
	c.items[cacheKey{subject: subject, kid: kid}] = cacheEntry{key: key, expires: now.Add(c.ttl)}
}

// evictLocked frees at least one slot: it deletes every expired entry and, if
// the cache is still at capacity, removes entries in (randomized) map-iteration
// order until below the cap. The caller must hold c.mu.
func (c *keyCache) evictLocked(now time.Time) {
	for k, e := range c.items {
		if !now.Before(e.expires) {
			delete(c.items, k)
		}
	}
	for k := range c.items {
		if len(c.items) < c.maxEntries {
			break
		}
		delete(c.items, k)
	}
}
