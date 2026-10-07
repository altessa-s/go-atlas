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
