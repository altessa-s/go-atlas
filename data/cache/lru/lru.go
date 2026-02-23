// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

import (
	"context"
	"fmt"
	"iter"

	"golang.org/x/sync/singleflight"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	lru "github.com/hashicorp/golang-lru/v2"
)

// Cacher defines the interface for LRU cache operations.
type Cacher[K comparable, V any] interface {
	// Get retrieves a value from the cache.
	Get(key K) (value V, ok bool)
	// Put adds or updates a value in the cache.
	Put(key K, value V) (evicted bool)
	// Remove removes a key from the cache.
	Remove(key K) (present bool)
	// Has checks if a key exists in the cache without affecting its position.
	Has(key K) bool
	// Len returns the number of items in the cache.
	Len() int
	// Purge removes all items from the cache.
	Purge()
	// Keys returns an iterator over all keys in the cache.
	Keys() iter.Seq[K]
	// All returns an iterator over all key-value pairs in the cache.
	All() iter.Seq2[K, V]
	// GetOrCompute retrieves a value or computes it using the provided function.
	// Uses singleflight to deduplicate concurrent computations for the same key.
	GetOrCompute(ctx context.Context, key K, fn func(ctx context.Context) (V, error)) (V, error)
}

// Cache is a thread-safe LRU cache.
type Cache[K comparable, V any] struct {
	cache *lru.Cache[K, V]
	group singleflight.Group
}

// NewCache creates a new thread-safe LRU cache with the given size.
func NewCache[K comparable, V any](size int) (*Cache[K, V], error) {
	cache, err := lru.New[K, V](size)
	if err != nil {
		return nil, err
	}
	return &Cache[K, V]{cache: cache}, nil
}

// Get returns the value for key and true if found, or the zero value and false.
func (sc *Cache[K, V]) Get(key K) (V, bool) {
	return sc.cache.Get(key)
}

// Put adds or updates key with value. Returns true if an eviction occurred.
func (sc *Cache[K, V]) Put(key K, value V) bool {
	return sc.cache.Add(key, value)
}

// Remove deletes key from the cache. Returns true if the key was present.
func (sc *Cache[K, V]) Remove(key K) bool {
	return sc.cache.Remove(key)
}

// Has reports whether key is present without updating recency.
func (sc *Cache[K, V]) Has(key K) bool {
	return sc.cache.Contains(key)
}

// Len returns the number of entries in the cache.
func (sc *Cache[K, V]) Len() int {
	return sc.cache.Len()
}

// Purge removes all entries from the cache.
func (sc *Cache[K, V]) Purge() {
	sc.cache.Purge()
}

// Keys returns an iterator over all keys in the cache.
func (sc *Cache[K, V]) Keys() iter.Seq[K] {
	return coreslices.Values(sc.cache.Keys())
}

// All returns an iterator over all key-value pairs in the cache.
func (sc *Cache[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for _, k := range sc.cache.Keys() {
			if v, ok := sc.cache.Peek(k); ok {
				if !yield(k, v) {
					return
				}
			}
		}
	}
}

// GetOrCompute returns the cached value for key, or calls fn exactly once
// (via singleflight) to compute and cache the result on a miss.
func (sc *Cache[K, V]) GetOrCompute(ctx context.Context, key K, fn func(ctx context.Context) (V, error)) (V, error) {
	if v, ok := sc.Get(key); ok {
		return v, nil
	}

	keyStr := fmt.Sprintf("%v", key)
	val, err, _ := sc.group.Do(keyStr, func() (any, error) {
		// Re-check cache inside singleflight
		if v, ok := sc.Get(key); ok {
			return v, nil
		}

		v, err := fn(ctx)
		if err != nil {
			return nil, err
		}

		sc.Put(key, v)
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
	_ Cacher[string, any] = (*Cache[string, any])(nil)
)
