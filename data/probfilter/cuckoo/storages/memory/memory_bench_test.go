// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"

	memory "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
)

func BenchmarkStorage_Add(b *testing.B) {
	storage := memory.New(memory.WithCapacity(100000))
	ctx := b.Context()

	b.ResetTimer()
	for b.Loop() {
		_ = storage.Add(ctx, "benchmark-value")
	}
}

func BenchmarkStorage_MightExist(b *testing.B) {
	storage := memory.New(memory.WithCapacity(100000))
	ctx := b.Context()

	_ = storage.Add(ctx, "benchmark-value")

	b.ResetTimer()
	for b.Loop() {
		_, _ = storage.MightExist(ctx, "benchmark-value")
	}
}

func BenchmarkStorage_Delete(b *testing.B) {
	storage := memory.New(memory.WithCapacity(100000))
	ctx := b.Context()

	for i := range 1000 {
		_ = storage.Add(ctx, string(rune('a'+i%26)))
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = storage.Delete(ctx, "a")
	}
}
