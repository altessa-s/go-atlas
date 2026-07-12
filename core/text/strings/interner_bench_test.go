// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings_test

import (
	"fmt"
	"testing"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// benchColdKeys returns n distinct pre-interned keys. With only
// [corestrings.HotCacheSlots] hot slots, rotating over thousands of keys
// keeps lookups on the cold-cache path — the one that bumps the shared
// promotion counter on every hit.
func benchColdKeys(si *corestrings.Interner, n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("interner-bench-key-%05d", i)
		si.String(keys[i])
	}
	return keys
}

// BenchmarkInterner_ColdHit pins the sequential cold-cache hit cost.
func BenchmarkInterner_ColdHit(b *testing.B) {
	si := corestrings.NewInterner(corestrings.DefaultMaxSize)
	keys := benchColdKeys(si, 4096)

	b.ReportAllocs()
	i := 0
	for b.Loop() {
		_ = si.String(keys[i%len(keys)])
		i++
	}
}

// BenchmarkInterner_ColdHitParallel exercises the same cold-cache path from
// concurrent goroutines. Run with -cpu 1,8 to expose contention on the
// process-wide promotion counter and the per-entry access atomics.
func BenchmarkInterner_ColdHitParallel(b *testing.B) {
	si := corestrings.NewInterner(corestrings.DefaultMaxSize)
	keys := benchColdKeys(si, 4096)

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = si.String(keys[i%len(keys)])
			i++
		}
	})
}
