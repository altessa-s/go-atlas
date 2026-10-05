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
// concurrent use. An expired entry is treated as a miss on read but is removed
// from the map only when a capacity-bound insert triggers eviction; a hard
// maxEntries cap then bounds memory so a churn of short-lived subjects (one-off
// principals, rotated-away kids) cannot grow the map without limit. A
// non-positive maxEntries disables the cap, leaving the cache unbounded.
//
// Every invalidation advances a cache-wide generation. A lookup records the
// generation before consulting the key provider and stores its result through
// putIfGeneration, so a lookup that overlapped an invalidation can never
// re-populate the cache with a key read before that invalidation. The
// generation is cache-wide rather than per subject so it needs no unbounded
// per-subject bookkeeping; the cost is that an overlapping lookup for an
// unrelated subject skips one cache populate.
type keyCache struct {
	ttl        time.Duration
	maxEntries int
	clock      Clock
	mu         sync.RWMutex
	items      map[cacheKey]cacheEntry
	gen        uint64
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

// generation returns the current invalidation generation. A lookup reads it
// before consulting the key provider and hands it to putIfGeneration.
func (c *keyCache) generation() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.gen
}

// putIfGeneration stores key for (subject, kid), expiring one TTL from now, but
// only when no invalidation has happened since gen was read: a result resolved
// under an older generation may carry a key retired by that invalidation, so it
// is dropped instead of cached. When the cache is at its size cap it first
// drops expired entries and, if that frees nothing, evicts arbitrary entries
// until a slot is free, keeping the map bounded.
func (c *keyCache) putIfGeneration(subject, kid string, key VerificationKey, gen uint64) {
	// A non-positive TTL disables caching: every lookup hits the KeyProvider, so
	// there is nothing to store (get would treat any entry as already expired).
	if c.ttl <= 0 {
		return
	}
	now := c.clock.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen {
		return
	}
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

// deleteKey removes the cached entry for (subject, kid) and advances the
// invalidation generation, fencing lookups already in flight.
func (c *keyCache) deleteKey(subject, kid string) {
	c.mu.Lock()
	c.gen++
	delete(c.items, cacheKey{subject: subject, kid: kid})
	c.mu.Unlock()
}

// deleteSubject removes every cached entry for subject regardless of kid and
// advances the invalidation generation, fencing lookups already in flight.
func (c *keyCache) deleteSubject(subject string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	for k := range c.items {
		if k.subject == subject {
			delete(c.items, k)
		}
	}
}
