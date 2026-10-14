// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testShardCounts covers the single-lock default and a sharded provider.
var testShardCounts = []int{1, 8}

// tracked reports whether key currently has a bucket.
func (p *Provider) tracked(key string) bool {
	sh := p.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	_, ok := sh.buckets[key]
	return ok
}

// total validates every shard and returns the number of buckets: map and
// list must be a bijection with matching nodes, recency and lastUsed must
// not increase toward the tail, and count must equal the bucket total.
func (p *Provider) total(tb testing.TB) int {
	tb.Helper()
	p.lockAll()
	defer p.unlockAll()
	n := 0
	for i := range p.shards {
		sh := &p.shards[i]
		require.Equal(tb, len(sh.buckets), sh.lru.Len())
		var prev *bucket
		for elem := sh.lru.Front(); elem != nil; elem = elem.Next() {
			key, ok := elem.Value.(string)
			require.True(tb, ok)
			b := sh.buckets[key]
			require.NotNil(tb, b, "list key %q missing from map", key)
			require.Same(tb, elem, b.lruElem)
			if prev != nil {
				require.Greater(tb, prev.recency, b.recency)
				require.False(tb, prev.lastUsed.Before(b.lastUsed))
			}
			prev = b
		}
		n += len(sh.buckets)
	}
	require.Equal(tb, int64(n), p.count.Load())
	return n
}

// keysInShards returns n keys from distinct shards when distinct is true,
// otherwise n keys from one shard.
func (p *Provider) keysInShards(n int, distinct bool) []string {
	var keys []string
	used := map[*shard]bool{}
	var first *shard
	for i := 0; len(keys) < n; i++ {
		k := "k" + strconv.Itoa(i)
		sh := p.shardFor(k)
		switch {
		case distinct && !used[sh]:
			used[sh] = true
			keys = append(keys, k)
		case !distinct && (first == nil || sh == first):
			first = sh
			keys = append(keys, k)
		}
	}
	return keys
}

func TestProvider_ConcurrentInsertsRespectCap(t *testing.T) {
	t.Parallel()
	const maxBuckets = 64
	for _, shards := range testShardCounts {
		t.Run("shards="+strconv.Itoa(shards), func(t *testing.T) {
			t.Parallel()
			concurrentInsertsRespectCap(t, New(WithMaxBuckets(maxBuckets), WithShards(shards)), maxBuckets)
		})
	}
}

func concurrentInsertsRespectCap(t *testing.T, p *Provider, maxBuckets int) {
	ctx := t.Context()

	var wg sync.WaitGroup
	for g := range 16 {
		wg.Go(func() {
			for i := range 500 {
				_, _ = p.Allow(ctx, "g"+strconv.Itoa(g)+"-"+strconv.Itoa(i), 10, time.Minute)
				// Re-touch an early key so hits interleave with inserts.
				_, _ = p.Allow(ctx, "g"+strconv.Itoa(g)+"-0", 1_000_000, time.Minute)
			}
		})
	}
	wg.Wait()
	require.Equal(t, maxBuckets, p.total(t))
}

func TestProvider_EvictionIsGlobalLRU(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	tests := []struct {
		name   string
		touch  func(t *testing.T, p *Provider, k []string)
		victim int // index into the first three keys
	}{
		{name: "oldest key goes first", touch: func(*testing.T, *Provider, []string) {}, victim: 0},
		{
			name: "allowed touch refreshes recency",
			touch: func(_ *testing.T, p *Provider, k []string) {
				_, _ = p.Allow(ctx, k[0], 1_000, time.Minute)
			},
			victim: 1,
		},
		{
			name: "rejected touch refreshes recency",
			touch: func(_ *testing.T, p *Provider, k []string) {
				_, _ = p.Allow(ctx, k[0], 1, time.Minute) // over the limit
			},
			victim: 1,
		},
		{
			name: "reset keeps recency",
			touch: func(t *testing.T, p *Provider, k []string) {
				require.NoError(t, p.Reset(ctx, k[0]))
			},
			victim: 0,
		},
	}
	for _, placement := range []struct {
		name     string
		distinct bool
	}{{"same shard", false}, {"across shards", true}} {
		for _, tc := range tests {
			t.Run(placement.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				p := New(WithMaxBuckets(3), WithShards(8))
				keys := p.keysInShards(4, placement.distinct)
				for _, k := range keys[:3] {
					_, err := p.Allow(ctx, k, 1, time.Minute)
					require.NoError(t, err)
				}
				tc.touch(t, p, keys)
				_, err := p.Allow(ctx, keys[3], 1, time.Minute)
				require.NoError(t, err)

				for i, k := range keys {
					require.Equal(t, i != tc.victim, p.tracked(k), "key %d (%s)", i, k)
				}
				require.Equal(t, 3, p.total(t))
			})
		}
	}
}

// TestProvider_ConcurrentSameKeyInsert pins that racing first requests for
// one key create one bucket and admit exactly limit requests, below the cap
// and at it.
func TestProvider_ConcurrentSameKeyInsert(t *testing.T) {
	t.Parallel()
	for _, maxBuckets := range []int{0, 1} {
		t.Run("maxBuckets="+strconv.Itoa(maxBuckets), func(t *testing.T) {
			t.Parallel()
			p := New(WithMaxBuckets(maxBuckets), WithShards(8))
			ctx := t.Context()
			if maxBuckets > 0 {
				_, err := p.Allow(ctx, "filler", 1, time.Minute) // fill the cap
				require.NoError(t, err)
			}

			var allowed atomic.Int64
			var wg sync.WaitGroup
			for range 32 {
				wg.Go(func() {
					if _, err := p.Allow(ctx, "same", 1, time.Minute); err == nil {
						allowed.Add(1)
					}
				})
			}
			wg.Wait()
			require.Equal(t, int64(1), allowed.Load())
			require.True(t, p.tracked("same"))
			require.Equal(t, max(maxBuckets, 1), p.total(t))
		})
	}
}

// TestProvider_ConcurrentMixedKeepsInvariants runs hits, inserts at the cap,
// resets and cleanup together and checks accounting while they run.
func TestProvider_ConcurrentMixedKeepsInvariants(t *testing.T) {
	t.Parallel()
	for _, shards := range testShardCounts {
		t.Run("shards="+strconv.Itoa(shards), func(t *testing.T) {
			t.Parallel()
			const maxBuckets = 32
			p := New(WithMaxBuckets(maxBuckets), WithMaxIdleTime(time.Millisecond), WithShards(shards))
			ctx := t.Context()

			stop := make(chan struct{})
			var wg sync.WaitGroup
			var ops, cleanups atomic.Int64
			t.Cleanup(func() {
				close(stop)
				wg.Wait()
			})
			for g := range 8 {
				wg.Go(func() {
					for i := 0; ; i++ {
						select {
						case <-stop:
							return
						default:
						}
						key := "g" + strconv.Itoa(g) + "-" + strconv.Itoa(i%64)
						_, _ = p.Allow(ctx, key, 1_000, time.Minute)
						if i%7 == 0 {
							_ = p.Reset(ctx, key)
						}
						ops.Add(1)
					}
				})
			}
			wg.Go(func() {
				for {
					select {
					case <-stop:
						return
					default:
						p.RunCleanup()
						cleanups.Add(1)
					}
				}
			})
			// Check while the workload is demonstrably running.
			for ops.Load() < 20_000 || cleanups.Load() < 50 {
				require.LessOrEqual(t, p.total(t), maxBuckets)
			}
		})
	}
}

// TestProvider_RunCleanupRemovesIdleKeepsTouched pins cleanup across shards:
// idle buckets go, a bucket touched after them stays, and the freed slots
// can be refilled up to the cap.
func TestProvider_RunCleanupRemovesIdleKeepsTouched(t *testing.T) {
	t.Parallel()
	for _, shards := range testShardCounts {
		t.Run("shards="+strconv.Itoa(shards), func(t *testing.T) {
			t.Parallel()
			const maxBuckets = 16
			p := New(WithMaxBuckets(maxBuckets), WithMaxIdleTime(20*time.Millisecond), WithShards(shards))
			ctx := t.Context()
			for i := range maxBuckets {
				_, err := p.Allow(ctx, "idle-"+strconv.Itoa(i), 10, time.Minute)
				require.NoError(t, err)
			}
			time.Sleep(40 * time.Millisecond)
			_, err := p.Allow(ctx, "idle-0", 10, time.Minute)
			require.NoError(t, err)

			p.RunCleanup()
			require.Equal(t, 1, p.total(t))
			require.True(t, p.tracked("idle-0"))

			for i := range maxBuckets - 1 {
				_, err := p.Allow(ctx, "fresh-"+strconv.Itoa(i), 10, time.Minute)
				require.NoError(t, err)
			}
			require.Equal(t, maxBuckets, p.total(t))
			require.True(t, p.tracked("idle-0"), "refill below the cap must not evict")
		})
	}
}

func TestProvider_CapOfOne(t *testing.T) {
	t.Parallel()
	for _, shards := range testShardCounts {
		t.Run("shards="+strconv.Itoa(shards), func(t *testing.T) {
			t.Parallel()
			p := New(WithMaxBuckets(1), WithShards(shards))
			ctx := t.Context()
			for i := range 10 {
				_, err := p.Allow(ctx, "k"+strconv.Itoa(i), 1, time.Minute)
				require.NoError(t, err)
				require.Equal(t, 1, p.total(t))
				require.True(t, p.tracked("k"+strconv.Itoa(i)))
			}
		})
	}
}

func TestProvider_ShardCountNormalization(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts []Option
		want int
	}{
		{name: "default", want: DefaultShards},
		{name: "zero", opts: []Option{WithShards(0)}, want: 1},
		{name: "negative", opts: []Option{WithShards(-3)}, want: 1},
		{name: "non power of two", opts: []Option{WithShards(3)}, want: 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := New(append(tc.opts, WithMaxBuckets(4))...)
			require.Len(t, p.shards, tc.want)
			ctx := t.Context()
			for i := range 10 {
				_, err := p.Allow(ctx, "k"+strconv.Itoa(i), 1, time.Minute)
				require.NoError(t, err)
			}
			require.Equal(t, 4, p.total(t))
		})
	}
}
