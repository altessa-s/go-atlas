// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings

import (
	"fmt"
	"sync"
	"testing"
)

// resetRegexCache clears the package-level case-insensitive regex cache and
// counter so each test starts from a known state. Test helpers in this file
// are white-box (package strings) to observe the counter directly.
func resetRegexCache(t testing.TB) {
	t.Helper()
	caseInsensitiveRegexCache.Range(func(k, _ any) bool {
		caseInsensitiveRegexCache.Delete(k)
		return true
	})
	regexCacheSize.Store(0)
}

// TestSplit_RegexCache_BoundHoldsUnderContention exercises the Split
// case-insensitive slow path concurrently with far more distinct separators
// than the cache bound. Before the fix, the Load-then-LoadOrStore pattern let
// concurrent goroutines each observe size < max and all increment the
// counter, so the number of cached entries could exceed maxRegexCacheSize.
//
// After the fix, the insert-then-rollback pattern is monotone: at any instant
// the size tracker must be <= maxRegexCacheSize, and every separator that
// made it into the sync.Map must be accounted for in the counter.
//
// The separator's lowercased form must have a different byte length than the
// original (Unicode special casing) to reach the cache path — we use the
// capital sharp S "ẞ" (U+1E9E), which lowercases to "ß" (U+00DF) and has a
// different UTF-8 length, as a prefix.
func TestSplit_RegexCache_BoundHoldsUnderContention(t *testing.T) {
	resetRegexCache(t)
	t.Cleanup(func() { resetRegexCache(t) })

	const (
		goroutines       = 32
		separatorsPerG   = 40
		totalDistinctSep = goroutines * separatorsPerG
	)
	if totalDistinctSep <= maxRegexCacheSize {
		t.Fatalf("test precondition: totalDistinctSep (%d) must exceed maxRegexCacheSize (%d)",
			totalDistinctSep, maxRegexCacheSize)
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	for g := range goroutines {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			<-start
			for i := range separatorsPerG {
				// "ẞ" forces the slow path (lowercased length differs).
				sep := fmt.Sprintf("ẞ-g%d-i%d", gid, i)
				_ = Split("a"+sep+"b"+sep+"c", SplitOptions{
					Separator:     sep,
					CaseSensitive: false,
				})
			}
		}(g)
	}
	close(start)
	wg.Wait()

	// Invariant 1: the counter never overshoots the bound.
	if got := regexCacheSize.Load(); got > maxRegexCacheSize {
		t.Errorf("regexCacheSize = %d, must not exceed maxRegexCacheSize = %d", got, maxRegexCacheSize)
	}

	// Invariant 2: the counter matches the actual number of map entries.
	var actual int64
	caseInsensitiveRegexCache.Range(func(_, _ any) bool {
		actual++
		return true
	})
	if actual > maxRegexCacheSize {
		t.Errorf("caseInsensitiveRegexCache holds %d entries, exceeds bound %d", actual, maxRegexCacheSize)
	}
	if actual != regexCacheSize.Load() {
		t.Errorf("counter drift: map has %d entries but counter reports %d",
			actual, regexCacheSize.Load())
	}
}

// BenchmarkSplit_CaseInsensitive_CacheHit exercises the hot path — the
// separator is already cached so only a sync.Map.Load runs. Useful for
// detecting regressions in the common case after the race fix.
func BenchmarkSplit_CaseInsensitive_CacheHit(b *testing.B) {
	resetRegexCache(b)
	b.Cleanup(func() { resetRegexCache(b) })

	const sep = "ẞ-cache-hit"
	input := "alpha" + sep + "beta" + sep + "gamma"
	opts := SplitOptions{Separator: sep, CaseSensitive: false}
	// Warm the cache.
	_ = Split(input, opts)

	b.ReportAllocs()
	for b.Loop() {
		_ = Split(input, opts)
	}
}
