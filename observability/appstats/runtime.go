// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats

import (
	"runtime"
	"sync"
)

var memStatsBuffer = sync.Pool{New: func() any { return &runtime.MemStats{} }}

// MemStats returns the current memory statistics for the Go runtime using object pooling.
// It uses a memory pool to reduce allocations and provides detailed heap, stack, and GC information.
// The function is optimized for frequent calls with minimal allocation overhead.
//
// This function calls runtime.ReadMemStats internally, which may cause a brief pause
// in garbage collection to ensure consistent results.
//
// Returns a runtime.MemStats struct containing detailed memory statistics,
// or nil if memory statistics cannot be retrieved.
func MemStats() *runtime.MemStats {
	if memStatsInterface := memStatsBuffer.Get(); memStatsInterface != nil {
		memStats, ok := memStatsInterface.(*runtime.MemStats)
		if !ok {
			// Fallback to direct allocation if type assertion fails
			result := &runtime.MemStats{}
			runtime.ReadMemStats(result)
			return result
		}

		runtime.ReadMemStats(memStats)

		// Create a copy since we're returning it
		result := &runtime.MemStats{}
		*result = *memStats

		// Return to pool
		memStatsBuffer.Put(memStats)
		return result
	}

	// Fallback if Get() returns nil
	result := &runtime.MemStats{}
	runtime.ReadMemStats(result)
	return result
}

// Runtime values are sampled on demand from a single memory snapshot.
func collectRuntime(stats *ApplicationStats) {
	memStats := MemStats()
	stats.Runtime.Goroutines = runtime.NumGoroutine()
	stats.Runtime.HeapAllocBytes = memStats.HeapAlloc
	stats.Runtime.HeapSysBytes = memStats.HeapSys
	stats.Runtime.HeapIdleBytes = memStats.HeapIdle
	stats.Runtime.HeapInUseBytes = memStats.HeapInuse
	stats.Runtime.HeapReleasedBytes = memStats.HeapReleased
	stats.Runtime.HeapObjects = memStats.HeapObjects
	stats.Runtime.GCCycles = memStats.NumGC
	stats.Runtime.GCPauseTotalNs = memStats.PauseTotalNs
	stats.Runtime.StackInUseBytes = memStats.StackInuse
	stats.Runtime.StackSysBytes = memStats.StackSys
}
