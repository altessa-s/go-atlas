// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package concurrency

import (
	"math"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultMemStatsCacheTTL is the default time-to-live for the cached
// runtime.MemStats snapshot used by [MemoryAwareConcurrency] and
// [AdaptiveConcurrency]. It balances data freshness against the cost
// of runtime.ReadMemStats, which triggers a stop-the-world pause.
// Adjust it with [SetMemStatsCacheTTL].
const DefaultMemStatsCacheTTL = time.Second

// cachedMemStats holds the cached memory statistics with TTL-based invalidation.
// This reduces the frequency of runtime.ReadMemStats calls, which cause
// stop-the-world pauses of 50-200μs each.
type cachedMemStats struct {
	stats       atomic.Pointer[runtime.MemStats]
	lastUpdated atomic.Int64 // Unix nanoseconds
	mu          sync.Mutex   // Protects update
	ttl         atomic.Int64 // TTL in nanoseconds
}

// globalMemStatsCache is the package-level cached memory stats.
var globalMemStatsCache = &cachedMemStats{}

func init() {
	globalMemStatsCache.ttl.Store(int64(DefaultMemStatsCacheTTL))
}

// getCachedMemStats returns cached memory statistics, refreshing if stale.
// This significantly reduces STW pauses by caching ReadMemStats results.
//
// The cache uses a TTL-based invalidation strategy:
//   - If cache is fresh (within TTL), returns cached values immediately
//   - If cache is stale, one goroutine refreshes while others get stale data
//   - This prevents thundering herd on cache expiration
func getCachedMemStats() *runtime.MemStats {
	return globalMemStatsCache.get()
}

// get returns the cached memory stats, refreshing if necessary.
// The returned pointer MUST be treated as read-only; it points directly into the cache
// to avoid copying the 4.8 KB runtime.MemStats struct on every call.
func (c *cachedMemStats) get() *runtime.MemStats {
	now := time.Now().UnixNano()
	lastUpdated := c.lastUpdated.Load()
	ttl := c.ttl.Load()

	// Fast path: get current pointer
	current := c.stats.Load()

	// Check if cache is still fresh and we have data
	if current != nil && lastUpdated > 0 && now-lastUpdated < ttl {
		return current
	}

	// Try to acquire the update lock
	// If another goroutine is updating, return stale data rather than wait
	if !c.mu.TryLock() {
		// Another goroutine is refreshing, return current (possibly stale) data
		if current != nil {
			return current
		}
		// If we have no data yet, we must wait for the lock
		c.mu.Lock()
	}
	defer c.mu.Unlock()

	// Double-check after acquiring lock (another goroutine might have updated)
	// Use fresh timestamp for accurate check
	nowAfterLock := time.Now().UnixNano()
	lastUpdated = c.lastUpdated.Load()
	current = c.stats.Load()
	ttl = c.ttl.Load()
	if current != nil && lastUpdated > 0 && nowAfterLock-lastUpdated < ttl {
		return current
	}

	// Refresh the cache
	// Allocate new struct to avoid modifying the one pointed to by atomic pointer
	newStats := new(runtime.MemStats)
	runtime.ReadMemStats(newStats)

	c.stats.Store(newStats)
	c.lastUpdated.Store(time.Now().UnixNano())

	return newStats
}

// SetMemStatsCacheTTL configures how long cached memory statistics remain valid
// before the next call to runtime.ReadMemStats. Shorter durations provide
// fresher data at the expense of more frequent stop-the-world pauses;
// longer durations reduce pauses but may cause concurrency decisions to use
// stale data.
//
// Recommended values:
//   - 100ms: latency-sensitive applications needing fresh memory readings
//   - 1s (default): good balance for most applications
//   - 5s: applications where memory pressure changes slowly
//
// Negative values are replaced with [DefaultMemStatsCacheTTL].
// SetMemStatsCacheTTL is safe for concurrent use.
func SetMemStatsCacheTTL(ttl time.Duration) {
	if ttl < 0 {
		ttl = DefaultMemStatsCacheTTL
	}
	globalMemStatsCache.ttl.Store(int64(ttl))
}

// InvalidateMemStatsCache marks the cached memory statistics as stale, causing
// the next concurrency-limit evaluation to call runtime.ReadMemStats. Use this
// after large allocations or deallocations where fresh data is critical.
// InvalidateMemStatsCache is safe for concurrent use.
func InvalidateMemStatsCache() {
	globalMemStatsCache.lastUpdated.Store(0)
}

// runtimeMemoryHeadroom measures the remaining Go runtime memory budget set
// through GOMEMLIMIT/debug.SetMemoryLimit. Runtime-managed memory is Sys minus
// HeapReleased, as defined by SetMemoryLimit; it is not machine free RAM.
func runtimeMemoryHeadroom() (uint64, bool) {
	limit := debug.SetMemoryLimit(-1)
	if limit == math.MaxInt64 {
		return 0, false
	}
	return memoryHeadroom(getCachedMemStats(), limit)
}

func memoryHeadroom(stats *runtime.MemStats, limit int64) (uint64, bool) {
	if limit < 0 || limit == math.MaxInt64 {
		return 0, false
	}
	used := stats.Sys - min(stats.Sys, stats.HeapReleased)
	if used >= uint64(limit) {
		return 0, true
	}
	return uint64(limit) - used, true
}

func getAvailableMemoryMB(source MemorySource) (uint64, bool) {
	available, known := source()
	return available / BytesToMB / BytesToMB, known
}
