// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

import (
	"context"
	"fmt"
	"iter"
	"time"

	"golang.org/x/sync/singleflight"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
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
type ExpirableCache[K comparable, V any] struct {
	cache *expirablelru.LRU[K, V]
	group singleflight.Group
}

// NewExpirableCache creates a cache holding at most size entries, each expiring
// ttl after it was added.
//
// A size of zero means unbounded (entries then leave only by expiry), and a ttl
// of zero disables expiry, which degrades this to a plain [Cache] — pass both
// as zero only if you meant to build a cache that never releases anything.
func NewExpirableCache[K comparable, V any](size int, ttl time.Duration) *ExpirableCache[K, V] {
	return &ExpirableCache[K, V]{
		cache: expirablelru.NewLRU[K, V](size, nil, ttl),
	}
}

// Get returns the value for key and true if found and unexpired, or the zero
// value and false.
func (c *ExpirableCache[K, V]) Get(key K) (V, bool) {
	return c.cache.Get(key)
}

// Put adds or updates key with value, restarting its TTL. Returns true if an
// eviction occurred.
func (c *ExpirableCache[K, V]) Put(key K, value V) bool {
	return c.cache.Add(key, value)
}

// Remove deletes key from the cache. Returns true if the key was present.
func (c *ExpirableCache[K, V]) Remove(key K) bool {
	return c.cache.Remove(key)
}

// Has reports whether key is present and unexpired, without updating recency.
func (c *ExpirableCache[K, V]) Has(key K) bool {
	return c.cache.Contains(key)
}

// Len returns the number of entries in the cache, including any that have
// expired but not yet been reaped.
func (c *ExpirableCache[K, V]) Len() int {
	return c.cache.Len()
}

// Purge removes all entries from the cache.
func (c *ExpirableCache[K, V]) Purge() {
	c.cache.Purge()
}

// Keys returns an iterator over all keys in the cache.
func (c *ExpirableCache[K, V]) Keys() iter.Seq[K] {
	return coreslices.Values(c.cache.Keys())
}

// All returns an iterator over all key-value pairs in the cache.
func (c *ExpirableCache[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for _, k := range c.cache.Keys() {
			if v, ok := c.cache.Peek(k); ok {
				if !yield(k, v) {
					return
				}
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

	keyStr := formatKey(key)
	val, err, _ := c.group.Do(keyStr, func() (any, error) {
		// Re-check inside singleflight: the winner may have just filled it.
		if v, ok := c.Get(key); ok {
			return v, nil
		}

		v, err := fn(ctx)
		if err != nil {
			return nil, err
		}

		c.Put(key, v)
		return v, nil
	})

	if err != nil {
		var zero V
		return zero, err
	}

	result, ok := val.(V)
	if !ok {
		var zero V
		return zero, fmt.Errorf("cache type assertion failed: got %T, expected %T", val, zero)
	}

	return result, nil
}

var (
	_ Cacher[string, any] = (*ExpirableCache[string, any])(nil)
)
