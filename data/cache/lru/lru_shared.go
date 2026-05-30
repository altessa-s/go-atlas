// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/fnv"
	"iter"
	"runtime"
	"sync"
	"unsafe"
)

// ShardedCache is a thread-safe, sharded LRU cache to reduce lock contention.
type ShardedCache[K comparable, V any] struct {
	shards     []*Cache[K, V]
	shardMask  uint64
	hasherPool sync.Pool
}

// NewShardedCache creates a new sharded LRU cache.
func NewShardedCache[K comparable, V any](totalSize int, opts ...Option) (*ShardedCache[K, V], error) {
	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	if o.shardCount <= 0 {
		o.shardCount = calculateOptimalShardCount()
	}

	// Ensure shard count is a power of 2
	if (o.shardCount & (o.shardCount - 1)) != 0 {
		o.shardCount = calculateOptimalShardCount()
	}

	shardSize := totalSize / o.shardCount
	if shardSize < 1 {
		shardSize = 1
	}

	shards := make([]*Cache[K, V], o.shardCount)
	for i := range o.shardCount {
		shard, err := NewCache[K, V](shardSize)
		if err != nil {
			return nil, err
		}
		shards[i] = shard
	}

	return &ShardedCache[K, V]{
		shards:    shards,
		shardMask: uint64(o.shardCount - 1), //nolint:gosec // G115: shardCount is validated positive and small (power of 2)
		hasherPool: sync.Pool{
			New: func() any {
				return fnv.New64a()
			},
		},
	}, nil
}

func (sc *ShardedCache[K, V]) getShard(key K) *Cache[K, V] {
	obj := sc.hasherPool.Get()
	hasher, ok := obj.(hash.Hash64)
	if !ok {
		// Should never happen
		return sc.shards[0]
	}
	defer sc.hasherPool.Put(hasher)
	hasher.Reset()

	switch k := any(key).(type) {
	case string:
		_, _ = hasher.Write(unsafe.Slice(unsafe.StringData(k), len(k))) // #nosec G103 -- zero-copy read-only access for hashing
	case []byte:
		_, _ = hasher.Write(k)
	case int:
		var buf [8]byte //nolint:mnd // Stack-allocated buffer for int-to-bytes hashing
		binary.LittleEndian.PutUint64(buf[:], uint64(int64(k)))
		_, _ = hasher.Write(buf[:])
	case int64:
		var buf [8]byte //nolint:mnd // Stack-allocated buffer for int64-to-bytes hashing
		binary.LittleEndian.PutUint64(buf[:], uint64(k))
		_, _ = hasher.Write(buf[:])
	case uint64:
		var buf [8]byte //nolint:mnd // Stack-allocated buffer for uint64-to-bytes hashing
		binary.LittleEndian.PutUint64(buf[:], k)
		_, _ = hasher.Write(buf[:])
	default:
		_, _ = hasher.Write(fmt.Appendf(nil, "%v", key))
	}

	return sc.shards[hasher.Sum64()&sc.shardMask]
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
