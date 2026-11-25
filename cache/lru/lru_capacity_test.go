// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

import (
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShardedCache_CollidingKeys(t *testing.T) {
	t.Parallel()

	c, err := NewShardedCache[int, int](100)
	require.NoError(t, err)

	const keyCount = 8
	// One shard must receive keyCount keys within this many attempts.
	buckets := make(map[*Cache[int, int]][]int)
	for key := range (keyCount-1)*len(c.shards) + 1 {
		shard := c.getShard(key)
		keys := append(buckets[shard], key)
		if len(keys) == keyCount {
			for _, key := range keys {
				require.False(t, c.Put(key, key), "evicted while inserting key %d", key)
			}
			require.Equal(t, keyCount, c.Len())
			return
		}
		buckets[shard] = keys
	}
	require.FailNow(t, "failed to find colliding keys")
}

func TestShardedCache_RandomKeys(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		capacity   int
		shardCount int
		keyCount   int
	}{
		{name: "small", capacity: 100, shardCount: 32, keyCount: 100},
		{name: "multiple shards", capacity: 257, shardCount: 4, keyCount: 64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			random := rand.New(rand.NewPCG(1, 2)) // Reproducible, distinct keys from a larger keyspace.
			c, err := NewShardedCache[int, int](tc.capacity, WithShardCount(tc.shardCount))
			require.NoError(t, err)

			for range 8 {
				keys := random.Perm(2048)[:tc.keyCount]
				for _, key := range keys {
					c.Put(key, key)
				}
				require.Equal(t, tc.keyCount, c.Len())
				for _, key := range keys {
					value, ok := c.Get(key)
					require.True(t, ok, "missing key %d", key)
					require.Equal(t, key, value)
				}
				c.Purge()
			}
		})
	}
}

func TestShardedCache_ShardCount(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		capacity  int
		requested int
		want      int
	}{
		{name: "zero capacity", capacity: 0, requested: 16, want: 1},
		{name: "negative capacity", capacity: -1, requested: 16, want: 1},
		{name: "small default", capacity: 100, want: 1},
		{name: "clamped power of two", capacity: 255, requested: 32, want: 2},
		{name: "explicit unchanged", capacity: 256, requested: 4, want: 4},
		{name: "explicit one shard", capacity: 256, requested: 1, want: 1},
		{name: "large default", capacity: 16384, want: calculateOptimalShardCount()},
		{name: "invalid uses default", capacity: 16384, requested: 3, want: calculateOptimalShardCount()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, err := NewShardedCache[int, int](tc.capacity, WithShardCount(tc.requested))
			require.NoError(t, err)
			require.Len(t, c.shards, tc.want)
		})
	}
}

func TestShardedCache_UsesRemainderCapacity(t *testing.T) {
	t.Parallel()

	const capacity = 257
	c, err := NewShardedCache[int, int](capacity, WithShardCount(4))
	require.NoError(t, err)

	for _, shard := range c.shards {
		for key := range capacity {
			if shard.Put(key, key) {
				break
			}
		}
	}
	require.Equal(t, capacity, c.Len())
}
