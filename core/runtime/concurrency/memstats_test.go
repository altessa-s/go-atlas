// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package concurrency

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestGetCachedMemStats_ReturnsSameDataWithinTTL(t *testing.T) {
	// Reset cache state
	InvalidateMemStatsCache()
	SetMemStatsCacheTTL(time.Second)

	// First call should populate cache
	stats1 := getCachedMemStats()
	if stats1 == nil {
		t.Fatal("getCachedMemStats returned nil")
	}

	// Second call within TTL should return cached data
	stats2 := getCachedMemStats()
	if stats2 == nil {
		t.Fatal("getCachedMemStats returned nil on second call")
	}

	// The Sys value should be the same (cached)
	if stats1.Sys != stats2.Sys {
		t.Errorf("Sys values differ: %d vs %d, expected same (cached)", stats1.Sys, stats2.Sys)
	}
}

func TestGetCachedMemStats_RefreshesAfterTTL(t *testing.T) {
	// Set a very short TTL for testing
	InvalidateMemStatsCache()
	SetMemStatsCacheTTL(10 * time.Millisecond)

	// First call
	stats1 := getCachedMemStats()
	if stats1 == nil {
		t.Fatal("getCachedMemStats returned nil")
	}

	// Wait for TTL to expire
	time.Sleep(20 * time.Millisecond)

	// Allocate some memory to potentially change stats
	data := make([]byte, 1024*1024) // 1MB
	_ = data

	// Second call should get fresh data
	stats2 := getCachedMemStats()
	if stats2 == nil {
		t.Fatal("getCachedMemStats returned nil after TTL")
	}

	// We can't guarantee values changed, but the call should succeed
	// and return valid data
	if stats2.Sys == 0 {
		t.Error("Sys should be non-zero")
	}

	// Reset TTL to default
	SetMemStatsCacheTTL(DefaultMemStatsCacheTTL)
}

func TestGetCachedMemStats_ConcurrentAccess(t *testing.T) {
	InvalidateMemStatsCache()
	SetMemStatsCacheTTL(50 * time.Millisecond)

	const numGoroutines = 100
	var wg sync.WaitGroup

	results := make(chan *runtime.MemStats, numGoroutines)

	for range numGoroutines {
		wg.Go(func() {
			stats := getCachedMemStats()
			results <- stats
		})
	}

	wg.Wait()
	close(results)

	// All results should be non-nil
	count := 0
	for stats := range results {
		if stats == nil {
			t.Error("got nil stats from concurrent call")
		}
		count++
	}

	if count != numGoroutines {
		t.Errorf("got %d results, want %d", count, numGoroutines)
	}

	// Reset TTL
	SetMemStatsCacheTTL(DefaultMemStatsCacheTTL)
}

func TestInvalidateMemStatsCache(t *testing.T) {
	InvalidateMemStatsCache()
	SetMemStatsCacheTTL(time.Hour) // Long TTL

	// First call populates cache
	stats1 := getCachedMemStats()
	if stats1 == nil {
		t.Fatal("getCachedMemStats returned nil")
	}

	// Invalidate cache
	InvalidateMemStatsCache()

	// Next call should refresh (though values might be same)
	stats2 := getCachedMemStats()
	if stats2 == nil {
		t.Fatal("getCachedMemStats returned nil after invalidation")
	}

	// Reset TTL
	SetMemStatsCacheTTL(DefaultMemStatsCacheTTL)
}

func TestGetAvailableMemoryMB(t *testing.T) {
	InvalidateMemStatsCache()

	memMB := getAvailableMemoryMB()

	// Available memory should be positive and reasonable
	// (at least some MB available, less than 1TB)
	if memMB == 0 {
		t.Error("available memory is 0, expected positive value")
	}
	if memMB > 1024*1024 { // 1TB
		t.Errorf("available memory %d MB seems unreasonably high", memMB)
	}
}

func TestSetMemStatsCacheTTL_NegativeValue(t *testing.T) {
	// Negative TTL should default to DefaultMemStatsCacheTTL
	SetMemStatsCacheTTL(-1)

	ttl := time.Duration(globalMemStatsCache.ttl.Load())
	if ttl != DefaultMemStatsCacheTTL {
		t.Errorf("TTL = %v, want %v (default)", ttl, DefaultMemStatsCacheTTL)
	}
}
