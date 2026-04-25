// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// cache_lru.go implements sharded LRU time cache using atomic operations.

package colorized

import (
	"sync"
	"sync/atomic"
	"time"
)

// hashString computes a 32-bit hash of a string using FNV-1a algorithm.
func hashString(s string) uint32 {
	hash := uint32(fnv1aOffset)
	for i := range len(s) {
		hash ^= uint32(s[i])
		hash *= fnv1aPrime
	}
	return hash
}

// shardedTimeCache is a time cache using sharding for better concurrency
type shardedTimeCache struct {
	shards    [16]*timeShard
	shardMask uint32
}

// timeShard represents a single shard of the cache
type timeShard struct {
	entries map[timeCacheKey]*timeCacheValue
	mu      sync.RWMutex
	lru     *simpleLRU
}

// timeCacheValue holds the cached formatted time
type timeCacheValue struct {
	formatted string
	lastUsed  atomic.Int64
}

// simpleLRU is a simple LRU tracker
type simpleLRU struct {
	order []timeCacheKey
	max   int
}

// newLockFreeTimeCacheV2 creates a new sharded time cache
func newShardedTimeCache(maxPerShard int) *shardedTimeCache {
	cache := &shardedTimeCache{
		//nolint:mnd
		shardMask: 15, // 16 shards - 1
	}

	for i := range cache.shards {
		cache.shards[i] = &timeShard{
			entries: make(map[timeCacheKey]*timeCacheValue),
			lru: &simpleLRU{
				order: make([]timeCacheKey, 0, maxPerShard),
				max:   maxPerShard,
			},
		}
	}

	return cache
}

// getShard returns the shard for a given key
func (c *shardedTimeCache) getShard(key timeCacheKey) *timeShard {
	hash := uint32(key.unix) ^ hashString(key.format) // #nosec G115 -- truncation is intentional for hash distribution
	return c.shards[hash&c.shardMask]
}

// get retrieves a value from the cache
func (c *shardedTimeCache) get(key timeCacheKey) (string, bool) {
	shard := c.getShard(key)

	shard.mu.RLock()
	if value, ok := shard.entries[key]; ok {
		formatted := value.formatted
		shard.mu.RUnlock()

		// Update last used time (non-blocking)
		value.lastUsed.Store(time.Now().UnixNano())
		return formatted, true
	}
	shard.mu.RUnlock()

	return "", false
}

// put adds a value to the cache
func (c *shardedTimeCache) put(key timeCacheKey, formatted string) {
	shard := c.getShard(key)
	now := time.Now().UnixNano()

	shard.mu.Lock()
	defer shard.mu.Unlock()

	// Check if already exists
	if value, ok := shard.entries[key]; ok {
		value.formatted = formatted
		value.lastUsed.Store(now)
		return
	}

	// Add new entry
	entry := &timeCacheValue{formatted: formatted}
	entry.lastUsed.Store(now)
	shard.entries[key] = entry

	// Track in LRU
	shard.lru.order = append(shard.lru.order, key)

	// Evict if needed
	if len(shard.lru.order) > shard.lru.max {
		oldest := shard.lru.order[0]
		delete(shard.entries, oldest)
		shard.lru.order = shard.lru.order[1:]
	}
}
