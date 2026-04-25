// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"iter"

	"github.com/altessa-s/go-atlas/data/cache/lru"
)

// Cache defines the interface for cache operations used by Manager.
// This abstraction allows switching between different cache implementations
// (e.g., regular LRU vs sharded cache) based on performance requirements.
type Cache[K comparable, V any] interface {
	// Get retrieves a value from the cache
	Get(key K) (value V, ok bool)
	// Put adds or updates a value in the cache
	Put(key K, value V) (evicted bool)
	// Remove removes a key from the cache
	Remove(key K) (present bool)
	// Has checks if a key exists in the cache
	Has(key K) bool
	// Len returns the number of items in the cache
	Len() int
	// Purge removes all items from the cache
	Purge()
	// Keys returns an iterator over all keys in the cache
	Keys() iter.Seq[K]
	// All returns an iterator over all key-value pairs in the cache
	All() iter.Seq2[K, V]
}

// StandardCache wraps a regular LRU cache to implement the Cache interface
type StandardCache[K comparable, V any] struct {
	lru.Cacher[K, V]
}

// NewStandardCache creates a new standard (non-sharded) cache
func NewStandardCache[K comparable, V any](size int) (*StandardCache[K, V], error) {
	cache, err := lru.NewCache[K, V](size)
	if err != nil {
		return nil, err
	}
	return &StandardCache[K, V]{Cacher: cache}, nil
}

// ShardedCache provides a thread-safe, sharded LRU cache to reduce lock contention
type ShardedCache[K comparable, V any] struct {
	lru.Cacher[K, V]
}

// ShardedCacheOption is a functional option for configuring ShardedCache
type ShardedCacheOption = lru.Option

// WithShardCount sets the number of shards (must be power of 2)
func WithShardCount(count int) ShardedCacheOption {
	return lru.WithShardCount(count)
}

// NewShardedCache creates a new sharded cache with the specified total size.
func NewShardedCache[K comparable, V any](totalSize int, options ...ShardedCacheOption) (*ShardedCache[K, V], error) {
	cache, err := lru.NewShardedCache[K, V](totalSize, options...)
	if err != nil {
		return nil, err
	}
	return &ShardedCache[K, V]{Cacher: cache}, nil
}
