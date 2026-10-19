// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

import (
	"context"
	"iter"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/singleflight"

	expirablelru "github.com/hashicorp/golang-lru/v2/expirable"
)

// ExpirableCache is a thread-safe LRU cache whose entries also expire after a
// fixed time-to-live.
//
// It bounds staleness as well as size: [Cache] only evicts when full, so a key
// that is written once and read forever never refreshes. Use this when an entry
// stops being true with age — a cached authorization decision, a resolved
// credential — rather than merely taking up room.
//
// The TTL is per entry and starts at insertion; reads do not extend it.
//
// Every entry records its own expiry time, and every operation judges expiry
// against it, by one clock, under the cache's lock: an entry is absent from the
// instant it expires for all readers and writers alike. Expired entries are
// freed by writes, with no background goroutine: a write sweeps them at most
// once per TTL, and always before inserting a new key into a full cache, so a
// live entry is never evicted while an expired one remains (the victim is the
// least recently used live entry); a write to the same key replaces one. Reads share the
// lock; writes (Put, Remove, Purge, PutIfAbsent, CompareAndSwap and the store
// of GetOrCompute) hold it exclusively, so the conditional writes are atomic
// with respect to every read, write and expiry.
type ExpirableCache[K comparable, V any] struct {
	cache *expirablelru.LRU[K, expiringEntry[V]]
	group *singleflight.Group[K, V]
	ttl   time.Duration
	now   func() time.Time
	size  int
	// nextSweep is when a write next frees the expired entries.
	nextSweep time.Time
	// earliest is a lower bound of the entries' expiry instants (exact after
	// a sweep): before it, no entry has expired and a sweep would find nothing.
	earliest time.Time
	// mu orders every operation against the conditional writes.
	mu sync.RWMutex
}

// expiringEntry is a cached value with the instant it expires; the zero
// expiresAt never expires (ttl 0).
type expiringEntry[V any] struct {
	value     V
	expiresAt time.Time
}

// NewExpirableCache creates a cache holding at most size entries, each expiring
// ttl after it was added.
//
// A size of zero means unbounded (entries then leave only by expiry), and a ttl
// of zero disables expiry, which degrades this to a plain [Cache] — pass both
// as zero only if you meant to build a cache that never releases anything.
func NewExpirableCache[K comparable, V any](size int, ttl time.Duration, opts ...ExpirableOption) *ExpirableCache[K, V] {
	o := newExpirableOptions(opts...)
	if o.clock == nil {
		o.clock = time.Now
	}
	return &ExpirableCache[K, V]{
		// No TTL underneath: expiry and cleanup follow the cache's own clock.
		cache: expirablelru.NewLRU[K, expiringEntry[V]](size, nil, 0),
		group: singleflight.New[K, V](singleflight.WithMaxConcurrent(0)),
		ttl:   ttl,
		size:  size,
		now:   o.clock,
	}
}

// entry returns key's entry if present and unexpired; the caller holds mu.
// touch updates recency.
func (c *ExpirableCache[K, V]) entry(key K, touch bool) (V, bool) {
	// Peek first, so an expired entry is never moved to the front, where it
	// would outlive live entries on eviction.
	e, ok := c.cache.Peek(key)
	if !ok || c.expired(e) {
		var zero V
		return zero, false
	}
	if touch {
		c.cache.Get(key)
	}
	return e.value, true
}

func (c *ExpirableCache[K, V]) expired(e expiringEntry[V]) bool {
	return !e.expiresAt.IsZero() && !c.now().Before(e.expiresAt)
}

// store adds key with a fresh expiry; the caller holds mu exclusively. When
// some entry may have expired, it first frees the expired entries if the
// periodic sweep is due or if inserting a new key into the full cache would
// otherwise evict an entry: the underlying LRU must only ever evict a live
// entry, and then the least recently used one.
func (c *ExpirableCache[K, V]) store(key K, value V) bool {
	e := expiringEntry[V]{value: value}
	if c.ttl > 0 {
		now := c.now()
		if !now.Before(c.earliest) && (!now.Before(c.nextSweep) || c.wouldEvict(key)) {
			c.sweep(now)
			c.nextSweep = now.Add(c.ttl)
		}
		e.expiresAt = now.Add(c.ttl)
		if c.earliest.IsZero() || e.expiresAt.Before(c.earliest) {
			c.earliest = e.expiresAt
		}
	}
	return c.cache.Add(key, e)
}

// wouldEvict reports whether adding key would make the underlying LRU evict.
func (c *ExpirableCache[K, V]) wouldEvict(key K) bool {
	if c.size <= 0 || c.cache.Len() < c.size {
		return false
	}
	_, present := c.cache.Peek(key)
	return !present
}

// sweep removes the entries expired at now and sets earliest to the exact
// minimum expiry of the rest; the caller holds mu exclusively. It costs one
// pass over the entries, and runs only when an entry may have expired: either
// it frees one, or it makes earliest exact, after which the next sweep waits
// until an entry has really expired (or its bound was a refreshed or removed
// entry's).
func (c *ExpirableCache[K, V]) sweep(now time.Time) {
	var earliest time.Time
	for _, k := range c.cache.Keys() {
		e, ok := c.cache.Peek(k)
		if !ok {
			continue
		}
		if !now.Before(e.expiresAt) {
			c.cache.Remove(k)
			continue
		}
		if earliest.IsZero() || e.expiresAt.Before(earliest) {
			earliest = e.expiresAt
		}
	}
	c.earliest = earliest
}

// Get returns the value for key and true if found and unexpired, or the zero
// value and false.
func (c *ExpirableCache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.entry(key, true)
}

// Put adds or updates key with value, restarting its TTL. Returns true if an
// eviction occurred.
func (c *ExpirableCache[K, V]) Put(key K, value V) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.store(key, value)
}

// PutIfAbsent stores value for key unless an unexpired entry exists, like
// [sync.Map.LoadOrStore]: it returns the existing value and true, or value and
// false after storing it (with a fresh TTL). The check and the store are
// atomic with respect to every other operation and to expiry.
func (c *ExpirableCache[K, V]) PutIfAbsent(key K, value V) (actual V, loaded bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.entry(key, true); ok {
		return v, true
	}
	c.store(key, value)
	return value, false
}

// CompareAndSwap stores newValue for key, restarting its TTL, if the key holds
// an unexpired entry equal to old, like [sync.Map.CompareAndSwap]; it reports
// whether it stored. The comparison is ==, so old must be of a comparable
// type (it panics otherwise). A missing or expired key never matches: an
// entry that any reader could already see as expired is never replaced. The
// expiry check, the comparison and the store are atomic with respect to every
// other operation.
func (c *ExpirableCache[K, V]) CompareAndSwap(key K, old, newValue V) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	current, ok := c.entry(key, false)
	if !ok || any(current) != any(old) {
		return false
	}
	c.store(key, newValue)
	return true
}

// Remove deletes key from the cache. Returns true if the key was present.
func (c *ExpirableCache[K, V]) Remove(key K) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cache.Remove(key)
}

// Has reports whether key is present and unexpired, without updating recency.
func (c *ExpirableCache[K, V]) Has(key K) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.entry(key, false)
	return ok
}

// Len returns the number of entries in the cache, including any that have
// expired but not yet been reaped.
func (c *ExpirableCache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cache.Len()
}

// Purge removes all entries from the cache.
func (c *ExpirableCache[K, V]) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache.Purge()
}

// Keys returns an iterator over the keys of the unexpired entries, as of the
// call.
func (c *ExpirableCache[K, V]) Keys() iter.Seq[K] {
	return func(yield func(K) bool) {
		for k := range c.All() {
			if !yield(k) {
				return
			}
		}
	}
}

// All returns an iterator over the unexpired key-value pairs, as of the call.
func (c *ExpirableCache[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		c.mu.RLock()
		keys := c.cache.Keys()
		type pair struct {
			k K
			v V
		}
		pairs := make([]pair, 0, len(keys))
		for _, k := range keys {
			if v, ok := c.entry(k, false); ok {
				pairs = append(pairs, pair{k, v})
			}
		}
		c.mu.RUnlock()
		for _, p := range pairs {
			if !yield(p.k, p.v) {
				return
			}
		}
	}
}

// GetOrCompute returns the cached value for key, or calls fn exactly once
// (via singleflight) to compute and cache the result on a miss or expiry.
func (c *ExpirableCache[K, V]) GetOrCompute(ctx context.Context, key K, fn func(ctx context.Context) (V, error)) (V, error) {
	if v, ok := c.Get(key); ok {
		return v, nil
	}
	return computeOnce(ctx, c.group, c.Get, c.Put, key, fn)
}

var (
	_ Cacher[string, any] = (*ExpirableCache[string, any])(nil)
)
