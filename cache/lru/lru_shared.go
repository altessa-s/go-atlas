// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

import (
	"context"
	"hash/maphash"
	"iter"
	"runtime"
)

// ShardedCache is a thread-safe, sharded LRU cache to reduce lock contention.
type ShardedCache[K comparable, V any] struct {
	shards    []*Cache[K, V]
	shardMask uint64
	seed      maphash.Seed
}

// NewShardedCache creates a sharded LRU cache with totalSize slots.
// A non-positive totalSize is treated as one.
//
// The shard count is a power of two and is reduced as needed to leave at
// least 64 slots per shard. Capacities below 128 use one shard, which holds
// the entire capacity. With s shards, an empty cache can accept at least
// totalSize/s distinct keys without eviction regardless of their hashes.
// Each shard applies its own LRU order and can evict while other shards
// still have room. See [WithShardCount] to request a shard count.
func NewShardedCache[K comparable, V any](totalSize int, opts ...Option) (*ShardedCache[K, V], error) {
	shardCount := newOptions(opts...).shardCount

	if shardCount <= 0 || shardCount&(shardCount-1) != 0 {
		shardCount = calculateOptimalShardCount()
	}

	totalSize = max(totalSize, 1)
	maxShards := max(totalSize/minShardSize, 1)
	for shardCount > maxShards {
		shardCount >>= 1
	}
	shardSize, remainder := totalSize/shardCount, totalSize%shardCount

	shards := make([]*Cache[K, V], shardCount)
	for i := range shards {
		size := shardSize
		if i < remainder {
			size++
		}
		shard, err := NewCache[K, V](size)
		if err != nil {
			return nil, err
		}
		shards[i] = shard
	}

	return &ShardedCache[K, V]{
		shards:    shards,
		shardMask: uint64(shardCount - 1), //nolint:gosec // G115: shardCount is always positive
		seed:      maphash.MakeSeed(),
	}, nil
}

func (sc *ShardedCache[K, V]) getShard(key K) *Cache[K, V] {
	return sc.shards[maphash.Comparable(sc.seed, key)&sc.shardMask]
}

// Get returns the value for key from the appropriate shard.
func (sc *ShardedCache[K, V]) Get(key K) (V, bool) {
	return sc.getShard(key).Get(key)
}

// Put adds or updates key in the appropriate shard.
func (sc *ShardedCache[K, V]) Put(key K, value V) bool {
	return sc.getShard(key).Put(key, value)
}

// Remove deletes key from the appropriate shard.
func (sc *ShardedCache[K, V]) Remove(key K) bool {
	return sc.getShard(key).Remove(key)
}

// Has reports whether key is present in any shard.
func (sc *ShardedCache[K, V]) Has(key K) bool {
	return sc.getShard(key).Has(key)
}

// Len returns the total number of entries across all shards.
func (sc *ShardedCache[K, V]) Len() int {
	total := 0
	for _, s := range sc.shards {
		total += s.Len()
	}
	return total
}

// Purge removes all entries from every shard.
func (sc *ShardedCache[K, V]) Purge() {
	for _, s := range sc.shards {
		s.Purge()
	}
}

// Keys returns an iterator over all keys across all shards.
func (sc *ShardedCache[K, V]) Keys() iter.Seq[K] {
	return func(yield func(K) bool) {
		for _, s := range sc.shards {
			for k := range s.Keys() {
				if !yield(k) {
					return
				}
			}
		}
	}
}

// All returns an iterator over all key-value pairs across all shards.
func (sc *ShardedCache[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for _, s := range sc.shards {
			for k, v := range s.All() {
				if !yield(k, v) {
					return
				}
			}
		}
	}
}

// GetOrCompute delegates to the appropriate shard's [Cache.GetOrCompute].
func (sc *ShardedCache[K, V]) GetOrCompute(ctx context.Context, key K, fn func(ctx context.Context) (V, error)) (V, error) {
	return sc.getShard(key).GetOrCompute(ctx, key, fn)
}

const (
	minShardSize       = 64
	minShardCount      = 16
	maxShardCount      = 256
	shardingMultiplier = 2
)

func calculateOptimalShardCount() int {
	n := runtime.NumCPU() * shardingMultiplier
	if n < minShardCount {
		return minShardCount
	}
	// Round up to power of 2
	p := 1
	for p < n {
		p <<= 1
	}
	if p > maxShardCount {
		return maxShardCount
	}
	return p
}

var _ Cacher[string, any] = (*ShardedCache[string, any])(nil)
