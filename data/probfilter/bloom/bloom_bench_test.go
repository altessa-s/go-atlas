// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bloom_test

import (
	"slices"
	"strconv"
	"testing"

	"github.com/altessa-s/go-atlas/data/probfilter/bloom"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/memory"
)

// benchItems is the number of pre-populated values used by the lookup benchmarks.
const benchItems = 10000

// benchKeys returns benchItems present keys and benchItems absent keys.
func benchKeys() (hits, misses []string) {
	hits = make([]string, benchItems)
	misses = make([]string, benchItems)
	for i := range benchItems {
		hits[i] = "present-" + strconv.Itoa(i)
		misses[i] = "absent-" + strconv.Itoa(i)
	}
	return hits, misses
}

// newBenchFilter builds a filter pre-populated with the hit keys.
func newBenchFilter(b *testing.B) (f *bloom.Filter, hits, misses []string) {
	b.Helper()

	hits, misses = benchKeys()
	f = bloom.New(memory.New(memory.WithExpectedItems(benchItems)))
	if err := f.AddBatch(b.Context(), slices.Values(hits)); err != nil {
		b.Fatalf("AddBatch: %v", err)
	}
	return f, hits, misses
}

func BenchmarkFilter_Add(b *testing.B) {
	keys, _ := benchKeys()
	f := bloom.New(memory.New(memory.WithExpectedItems(benchItems)))
	ctx := b.Context()

	b.ReportAllocs()
	i := 0
	for b.Loop() {
		_ = f.Add(ctx, keys[i%benchItems])
		i++
	}
}

func BenchmarkFilter_MightExist_Hit(b *testing.B) {
	f, hits, _ := newBenchFilter(b)
	ctx := b.Context()

	b.ReportAllocs()
	i := 0
	for b.Loop() {
		_, _ = f.MightExist(ctx, hits[i%benchItems])
		i++
	}
}

func BenchmarkFilter_MightExist_Miss(b *testing.B) {
	f, _, misses := newBenchFilter(b)
	ctx := b.Context()

	b.ReportAllocs()
	i := 0
	for b.Loop() {
		_, _ = f.MightExist(ctx, misses[i%benchItems])
		i++
	}
}

// BenchmarkFilter_MightExist_Parallel measures concurrent lookups against a
// pre-populated filter with a 50/50 hit/miss mix. The in-memory storage guards
// its bit array with a sync.RWMutex, so this benchmark is the baseline for the
// planned RWMutex-contention investigation.
func BenchmarkFilter_MightExist_Parallel(b *testing.B) {
	f, hits, misses := newBenchFilter(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := hits[i%benchItems]
			if i&1 == 1 {
				key = misses[i%benchItems]
			}
			_, _ = f.MightExist(ctx, key)
			i++
		}
	})
}
