// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"strconv"
	"testing"

	memory "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/memory"
)

func BenchmarkStorage_Add(b *testing.B) {
	storage := memory.New(memory.WithExpectedItems(100000))
	ctx := b.Context()

	b.ResetTimer()
	for b.Loop() {
		_ = storage.Add(ctx, "benchmark-value")
	}
}

func BenchmarkStorage_MightExist(b *testing.B) {
	storage := memory.New(memory.WithExpectedItems(100000))
	ctx := b.Context()

	_ = storage.Add(ctx, "benchmark-value")

	b.ResetTimer()
	for b.Loop() {
		_, _ = storage.MightExist(ctx, "benchmark-value")
	}
}

func BenchmarkStorage_MightExist_Parallel(b *testing.B) {
	storage := memory.New(memory.WithExpectedItems(100000))
	ctx := b.Context()

	_ = storage.Add(ctx, "benchmark-value")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = storage.MightExist(ctx, "benchmark-value")
		}
	})
}

func BenchmarkStorage_MightExist_Parallel_MultiKey(b *testing.B) {
	storage := memory.New(memory.WithExpectedItems(100000))
	ctx := b.Context()

	keys := make([]string, 1024)
	for i := range keys {
		keys[i] = "benchmark-key-" + strconv.Itoa(i)
		_ = storage.Add(ctx, keys[i])
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var idx int
		for pb.Next() {
			_, _ = storage.MightExist(ctx, keys[idx&(len(keys)-1)])
			idx++
		}
	})
}
