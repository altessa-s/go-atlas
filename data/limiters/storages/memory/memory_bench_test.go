// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"strconv"
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
