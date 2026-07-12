// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package concurrency_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
)

func noopProcess(_ context.Context, _ int) error { return nil }

func transformDouble(_ context.Context, v int) (int, error) { return v * 2, nil }

func makeItems(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func BenchmarkProcessEmpty(b *testing.B) {
	ctx := b.Context()
	for b.Loop() {
		_ = concurrency.Process(ctx, nil, noopProcess, concurrency.WithConcurrency[int](4))
	}
}

func BenchmarkProcessSmallSequential(b *testing.B) {
	ctx := b.Context()
	items := makeItems(8)
	for b.Loop() {
		_ = concurrency.Process(ctx, items, noopProcess, concurrency.WithConcurrency[int](1))
	}
}

func BenchmarkProcessSmallParallel(b *testing.B) {
	ctx := b.Context()
	items := makeItems(64)
	for b.Loop() {
		_ = concurrency.Process(ctx, items, noopProcess, concurrency.WithConcurrency[int](4))
	}
}

// BenchmarkProcessLargeBatch exercises the case the worker pool exists for:
// item count far above the concurrency limit, where per-item goroutines
// would dominate the cost.
func BenchmarkProcessLargeBatch(b *testing.B) {
	ctx := b.Context()
	items := makeItems(10000)

	b.ReportAllocs()
	for b.Loop() {
		_ = concurrency.Process(ctx, items, noopProcess, concurrency.WithConcurrency[int](8))
	}
}

func BenchmarkProcessCollectLargeBatch(b *testing.B) {
	ctx := b.Context()
	items := makeItems(10000)

	b.ReportAllocs()
	for b.Loop() {
		_, _ = concurrency.ProcessCollect(ctx, items, transformDouble, concurrency.WithConcurrency[int](8))
	}
}

func BenchmarkProcessCollectSequential(b *testing.B) {
	ctx := b.Context()
	items := makeItems(8)
	for b.Loop() {
		_, _ = concurrency.ProcessCollect(ctx, items, transformDouble, concurrency.WithConcurrency[int](1))
	}
}

func BenchmarkProcessCollectParallel(b *testing.B) {
	ctx := b.Context()
	items := makeItems(64)
	for b.Loop() {
		_, _ = concurrency.ProcessCollect(ctx, items, transformDouble, concurrency.WithConcurrency[int](4))
	}
}
