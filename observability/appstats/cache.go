// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/altessa-s/go-atlas-core/runtime/concurrency"
)

// CachedMetrics holds cached system metrics for high-performance access.
// It uses atomic operations for lock-free reads and is safe for concurrent access.
// All metrics are updated by a background goroutine at regular intervals.
type CachedMetrics struct {
	// ServiceCPU contains the service CPU usage as a formatted string with '%' suffix
	ServiceCPU atomic.Value // string
	// SystemCPU contains the system CPU usage as a formatted string with '%' suffix
	SystemCPU atomic.Value // string

	// MemoryData contains complete memory statistics using atomic pointer for lock-free access
	MemoryData atomic.Pointer[MemoryLoadData]

	// NetworkReceived contains total bytes received since process start
	NetworkReceived atomic.Uint64
	// NetworkSent contains total bytes sent since process start
	NetworkSent atomic.Uint64

	// ProcessCache contains cached process object to avoid expensive OS calls
	ProcessCache atomic.Pointer[process.Process]

	// CPUTimestamp contains the Unix timestamp of the last CPU metrics update
	CPUTimestamp atomic.Int64
	// MemoryTimestamp contains the Unix timestamp of the last memory metrics update
	MemoryTimestamp atomic.Int64
	// NetworkTimestamp contains the Unix timestamp of the last network metrics update
	NetworkTimestamp atomic.Int64
	// ProcessTimestamp contains the Unix timestamp of the last process cache update
	ProcessTimestamp atomic.Int64
}

var cachedMetrics = &CachedMetrics{}

// Bind each collector to the process-wide cache once for all sampling ticks.
var metricCollectionTasks = []func(context.Context, int64){
	cachedMetrics.updateCPU,
	func(ctx context.Context, timestamp int64) {
		cachedMetrics.publishMemory(collectMemory(ctx, cachedMetrics.ProcessCache.Load), timestamp)
	},
	cachedMetrics.updateNetwork,
}

func (m *CachedMetrics) initialize() {
	m.ServiceCPU.Store(zeroPercent)
	m.SystemCPU.Store(zeroPercent)
	m.MemoryData.Store(&MemoryLoadData{
		Service: zeroPercent,
		System:  zeroPercent,
		Total:   "0",
	})
}

func collectMetrics(ctx context.Context) {
	now := time.Now().Unix()
	// Each collector publishes successful readings independently. Cancellation
	// leaves the last available samples in the cache.
	_ = concurrency.Process(ctx, metricCollectionTasks, func(ctx context.Context, collect func(context.Context, int64)) error {
		collect(ctx, now)
		return nil
	})
}

func (m *CachedMetrics) updateCPU(ctx context.Context, timestamp int64) {
	// Use cached process if available and recent
	var proc *process.Process
	if cached := m.ProcessCache.Load(); cached != nil {
		if time.Now().Unix()-m.ProcessTimestamp.Load() < ProcessCacheSeconds {
			proc = cached
		}
	}

	if proc == nil {
		var err error
		proc, err = ProcessDetails(ctx)
		if err != nil {
			return
		}
		m.ProcessCache.Store(proc)
		m.ProcessTimestamp.Store(timestamp)
	}

	if value, ok := collectServiceCPU(proc); ok {
		m.ServiceCPU.Store(value)
	}
	if value, ok := collectSystemCPU(ctx); ok {
		m.SystemCPU.Store(value)
	}
	m.CPUTimestamp.Store(timestamp)
}

func (m *CachedMetrics) publishMemory(data *MemoryLoadData, timestamp int64) {
	if data != nil {
		m.MemoryData.Store(data)
		m.MemoryTimestamp.Store(timestamp)
	}
}

func (m *CachedMetrics) updateNetwork(_ context.Context, timestamp int64) {
	received, sent, err := collectNetwork()
	if err == nil {
		m.NetworkReceived.Store(received)
		m.NetworkSent.Store(sent)
		m.NetworkTimestamp.Store(timestamp)
	}
}

// IsCacheStale checks if cached metrics are too old.
// It returns true if any of the CPU, memory, or network metrics are older than MaxCacheAge.
// This function is useful for determining if metrics should be refreshed or marked as stale.
//
// Returns true if cached data is considered stale, false otherwise.
func IsCacheStale() bool {
	now := time.Now().Unix()
	cpuAge := now - cachedMetrics.CPUTimestamp.Load()
	memAge := now - cachedMetrics.MemoryTimestamp.Load()
	netAge := now - cachedMetrics.NetworkTimestamp.Load()

	maxAge := int64(MaxCacheAge.Seconds())
	return cpuAge > maxAge || memAge > maxAge || netAge > maxAge
}
