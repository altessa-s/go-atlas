// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/limiters/storages/memory"
)

func BenchmarkProvider_Allow(b *testing.B) {
	p := memory.New()
	ctx := b.Context()

	for b.Loop() {
		_, _ = p.Allow(ctx, "bench-key", 1000000, time.Minute)
	}
}

// BenchmarkProvider_Allow_ChurnAtCapacity measures Allow for never-seen keys
// once the bucket cap is reached, so every call evicts a bucket.
func BenchmarkProvider_Allow_ChurnAtCapacity(b *testing.B) {
	for _, capacity := range []int{1_000, 10_000, 100_000} {
		b.Run(strconv.Itoa(capacity), func(b *testing.B) {
			p := memory.New(memory.WithMaxBuckets(capacity))
			ctx := b.Context()
			for i := range capacity {
				_, _ = p.Allow(ctx, "prefill-"+strconv.Itoa(i), 10, time.Minute)
			}

			i := 0
			for b.Loop() {
				_, _ = p.Allow(ctx, "churn-"+strconv.Itoa(i), 10, time.Minute)
				i++
			}
		})
	}
}

func BenchmarkProvider_Reset(b *testing.B) {
	p := memory.New()
	ctx := b.Context()
	_, _ = p.Allow(ctx, "bench-key", 1000000, time.Minute)

	for b.Loop() {
		_ = p.Reset(ctx, "bench-key")
	}
}

// BenchmarkProvider_RunCleanup measures a cleanup cycle over a full provider
// where no bucket is idle yet — the steady state of a scheduled sweep.
func BenchmarkProvider_RunCleanup(b *testing.B) {
	for _, size := range []int{10_000, 100_000} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			p := memory.New(memory.WithMaxBuckets(size), memory.WithMaxIdleTime(time.Hour))
			ctx := b.Context()
			for i := range size {
				_, _ = p.Allow(ctx, "key-"+strconv.Itoa(i), 10, time.Minute)
			}

			b.ReportAllocs()
			for b.Loop() {
				p.RunCleanup()
			}
		})
	}
}

// BenchmarkProvider_Allow_Parallel measures Allow under concurrent callers
// spread over many keys, exposing provider lock contention.
func BenchmarkProvider_Allow_Parallel(b *testing.B) {
	const keys = 1024
	p := memory.New()
	ctx := b.Context()
	names := make([]string, keys)
	for i := range names {
		names[i] = "key-" + strconv.Itoa(i)
	}

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = p.Allow(ctx, names[i%keys], 1_000_000, time.Minute)
			i++
		}
	})
}

// BenchmarkProvider_Allow_ParallelSpread is the steady-state per-request
// shape: workers start at different keys and a short window keeps request
// history bounded, so the numbers reflect lock contention, not growth.
func BenchmarkProvider_Allow_ParallelSpread(b *testing.B) {
	for _, shards := range benchShardCounts {
		b.Run("shards="+strconv.Itoa(shards), func(b *testing.B) {
			benchAllowParallelSpread(b, memory.New(memory.WithShards(shards)))
		})
	}
}

// benchShardCounts compares the single-lock default with sharded providers.
var benchShardCounts = []int{1, 8, 16}

func benchAllowParallelSpread(b *testing.B, p *memory.Provider) {
	const keys = 4096
	ctx := b.Context()
	names := make([]string, keys)
	for i := range names {
		names[i] = "key-" + strconv.Itoa(i)
	}
	var worker atomic.Int64

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := int(worker.Add(1)) * 7919
		for pb.Next() {
			_, _ = p.Allow(ctx, names[i%keys], 1_000_000, 10*time.Millisecond)
			i++
		}
	})
}

// BenchmarkProvider_Allow_ParallelHotKey hammers one key from every worker,
// the worst case for any per-key or per-shard lock.
func BenchmarkProvider_Allow_ParallelHotKey(b *testing.B) {
	p := memory.New()
	ctx := b.Context()

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = p.Allow(ctx, "hot", 1_000_000, 10*time.Millisecond)
		}
	})
}

// BenchmarkProvider_Allow_ParallelMixedAtCapacity mixes hits with new keys
// on a full provider (9 hits : 1 insert), so every insert evicts while other
// workers keep hitting their own keys.
func BenchmarkProvider_Allow_ParallelMixedAtCapacity(b *testing.B) {
	for _, shards := range benchShardCounts {
		b.Run("shards="+strconv.Itoa(shards), func(b *testing.B) {
			benchAllowParallelMixedAtCapacity(b, shards)
		})
	}
}

func benchAllowParallelMixedAtCapacity(b *testing.B, shards int) {
	const capacity = 10_000
	p := memory.New(memory.WithMaxBuckets(capacity), memory.WithShards(shards))
	ctx := b.Context()
	for i := range capacity {
		_, _ = p.Allow(ctx, "prefill-"+strconv.Itoa(i), 1_000_000, 10*time.Millisecond)
	}
	var worker atomic.Int64

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		w := strconv.Itoa(int(worker.Add(1)))
		hot := make([]string, 64)
		for i := range hot {
			hot[i] = "hot-" + w + "-" + strconv.Itoa(i)
		}
		for i := 0; pb.Next(); i++ {
			key := hot[i%len(hot)]
			if i%10 == 9 {
				key = "new-" + w + "-" + strconv.Itoa(i)
			}
			_, _ = p.Allow(ctx, key, 1_000_000, 10*time.Millisecond)
		}
	})
}
