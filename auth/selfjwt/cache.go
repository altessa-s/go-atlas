// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/cache/lru"

	jwt "github.com/altessa-s/go-atlas/auth/jwt"
)

// cacheKey identifies a cached verification key by subject and key id.
type cacheKey struct {
	subject string
	kid     string
}

// cacheEntry is a cached verification key, or a cached key-provider miss
// when err is non-nil, with its absolute expiry.
type cacheEntry struct {
	key     jwt.VerificationKey
	err     error
	expires time.Time
}

// negativeShare is the fraction of the size cap (one in negativeShare) the
// negative cache may hold.
const negativeShare = 10

// keyCache caches resolved verification keys by (subject, kid) with a TTL so
// the verifier avoids a key-provider lookup on every request. It is safe for
// concurrent use.
//
// Verified keys and key-provider misses live in two separate LRU caches. The
// miss cache is a fraction of the size cap, so tokens carrying random subjects
// or kids, which fill it with short-lived negative entries, can only evict
// other misses and never a key a legitimate subject needs. Both caches evict
// the least recently used entry in constant time when full; an expired entry is
// treated as a miss on read and leaves the cache when it is evicted or
// replaced, so memory stays bounded by the caps.
//
// Every invalidation advances a cache-wide generation. A lookup records the
// generation before consulting the key provider and stores its result through
// putIfGeneration, so a lookup that overlapped an invalidation can never
// re-populate the cache with a key read before that invalidation. The
// generation is cache-wide rather than per subject so it needs no unbounded
// per-subject bookkeeping; the cost is that an overlapping lookup for an
// unrelated subject skips one cache populate.
type keyCache struct {
	ttl       time.Duration
	negTTL    time.Duration
	clock     Clock
	positives *lru.Cache[cacheKey, cacheEntry]
	negatives *lru.Cache[cacheKey, cacheEntry]

	mu  sync.RWMutex // guards gen and orders every store against invalidations
	gen uint64
}

// newKeyCache builds an empty key cache with the given key TTL, miss TTL,
// size cap, and clock. A non-positive maxEntries selects [DefaultCacheMaxEntries]:
// the cache is always bounded.
func newKeyCache(ttl, negTTL time.Duration, maxEntries int, clock Clock) *keyCache {
	if maxEntries <= 0 {
		maxEntries = DefaultCacheMaxEntries
	}
	// The sizes are positive, so NewCache cannot fail.
	positives, _ := lru.NewCache[cacheKey, cacheEntry](maxEntries)
	negatives, _ := lru.NewCache[cacheKey, cacheEntry](max(1, maxEntries/negativeShare))
	return &keyCache{ttl: ttl, negTTL: negTTL, clock: clock, positives: positives, negatives: negatives}
}

// size reports the number of cached entries, positive and negative.
func (c *keyCache) size() int {
	return c.positives.Len() + c.negatives.Len()
}

// get returns the cached key for (subject, kid) when present and unexpired.
func (c *keyCache) get(subject, kid string) (jwt.VerificationKey, bool) {
	e, ok := c.fresh(c.positives, cacheKey{subject: subject, kid: kid})
	return e.key, ok
}

// getMiss returns the cached key-provider error for (subject, kid) when
// present and unexpired, and nil otherwise.
func (c *keyCache) getMiss(subject, kid string) error {
	e, _ := c.fresh(c.negatives, cacheKey{subject: subject, kid: kid})
	return e.err
}

func (c *keyCache) fresh(from *lru.Cache[cacheKey, cacheEntry], k cacheKey) (cacheEntry, bool) {
	e, ok := from.Get(k)
	if !ok || !c.clock.Now().Before(e.expires) {
		return cacheEntry{}, false
	}
	return e, true
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
// is dropped instead of cached. At capacity the least recently used key is
// evicted.
func (c *keyCache) putIfGeneration(subject, kid string, key jwt.VerificationKey, gen uint64) {
	c.storeIfGeneration(c.positives, cacheKey{subject: subject, kid: kid}, cacheEntry{key: key}, c.ttl, gen)
}

// putMissIfGeneration caches a key-provider miss for (subject, kid) for the
// miss TTL, under the same generation fence as putIfGeneration, in the
// separate, smaller miss cache.
func (c *keyCache) putMissIfGeneration(subject, kid string, err error, gen uint64) {
	c.storeIfGeneration(c.negatives, cacheKey{subject: subject, kid: kid}, cacheEntry{err: err}, c.negTTL, gen)
}

func (c *keyCache) storeIfGeneration(to *lru.Cache[cacheKey, cacheEntry], k cacheKey, e cacheEntry, ttl time.Duration, gen uint64) {
	// A non-positive TTL disables caching: every lookup hits the KeyProvider, so
	// there is nothing to store (fresh would treat any entry as already expired).
	if ttl <= 0 {
		return
	}
	now := c.clock.Now()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.gen != gen {
		return
	}
	e.expires = now.Add(ttl)
	to.Put(k, e)
}

// deleteKey removes the cached entry for (subject, kid) and advances the
// invalidation generation, fencing lookups already in flight.
func (c *keyCache) deleteKey(subject, kid string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	k := cacheKey{subject: subject, kid: kid}
	c.positives.Remove(k)
	c.negatives.Remove(k)
}

// deleteSubject removes every cached entry for subject regardless of kid and
// advances the invalidation generation, fencing lookups already in flight.
func (c *keyCache) deleteSubject(subject string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	for _, from := range [...]*lru.Cache[cacheKey, cacheEntry]{c.positives, c.negatives} {
		for k := range from.Keys() {
			if k.subject == subject {
				from.Remove(k)
			}
		}
	}
}
