// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"strconv"
	"sync/atomic"
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

// BenchmarkStorage_MixedParallel measures lock contention under a mixed
// workload: 80% MightExist, 10% Add, 10% Delete of the value just added, so
// the item count stays bounded. Compare ns/op across -cpu values.
func BenchmarkStorage_MixedParallel(b *testing.B) {
	storage := memory.New(memory.WithCapacity(100000))
	ctx := b.Context()

	keys := make([]string, 4096)
	extra := make([]string, len(keys))
	for i := range keys {
		keys[i] = "key-" + strconv.Itoa(i)
		extra[i] = "mixed-" + strconv.Itoa(i)
		if i%2 == 0 {
			_ = storage.Add(ctx, keys[i])
		}
	}

	var seed atomic.Uint64
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := int(seed.Add(1) * 7919)
		var added string
		for pb.Next() {
			i++
			k := keys[i%len(keys)]
			switch i % 10 {
			case 0:
				added = extra[i%len(extra)]
				_ = storage.Add(ctx, added)
			case 5:
				if added != "" {
					_, _ = storage.Delete(ctx, added)
				}
			default:
				_, _ = storage.MightExist(ctx, k)
			}
		}
	})
}
