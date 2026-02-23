// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// CPULoadData represents CPU load statistics for both the service and the system.
// It contains percentage values for CPU usage in string format for display purposes.
// All percentage values include the '%' suffix.
type CPULoadData struct {
	// Service is the CPU usage of the current service process as a percentage
	Service string
	// System is the total system CPU usage as a percentage
	System string
	// Total is the total system CPU usage as a percentage (same as System)
	Total string
}

// MemoryLoadData represents memory usage statistics for both the service and the system.
// It contains memory usage values in string format with appropriate units.
// Service and System values include the '%' suffix, Total is in bytes.
type MemoryLoadData struct {
	// Service is the memory usage of the current service process as a percentage
	Service string
	// System is the total system memory usage as a percentage
	System string
	// Total is the total available system memory in bytes
	Total string
}

// NetworkIOData represents network I/O statistics.
// It contains cumulative byte counts since the process started.
type NetworkIOData struct {
	// BytesReceived is the total bytes received since process started
	BytesReceived float64
	// BytesSent is the total bytes sent since process started
	BytesSent float64
}

// ApplicationStats represents comprehensive application statistics.
// It provides detailed information about process, CPU, memory, network, and runtime metrics.
// All statistics are collected using optimized background caching for minimal performance impact.
type ApplicationStats struct {
	// ProcessID is the operating system process identifier
	ProcessID int32 `json:"process_id"`
	// Uptime is the duration since the application started
	Uptime time.Duration `json:"uptime"`

	// CPU contains CPU usage statistics
	CPU struct {
		// Service is the CPU usage of the current service process as percentage
		Service string `json:"service"`
		// System is the total system CPU usage as percentage
		System string `json:"system"`
	} `json:"cpu"`

	// Memory contains memory usage statistics
	Memory struct {
		// Service is the memory usage of the current service process as percentage
		Service string `json:"service"`
		// System is the total system memory usage as percentage
		System string `json:"system"`
		// Total is the total system memory available in bytes
		Total string `json:"total"`
	} `json:"memory"`

	// Network contains network I/O statistics
	Network struct {
		// BytesReceived is formatted bytes received with units
		BytesReceived string `json:"bytes_received"`
		// BytesSent is formatted bytes sent with units
		BytesSent string `json:"bytes_sent"`
		// RawReceived is the raw bytes received as float64
		RawReceived float64 `json:"raw_received"`
		// RawSent is the raw bytes sent as float64
		RawSent float64 `json:"raw_sent"`
	} `json:"network"`

	// Runtime contains Go runtime statistics
	Runtime struct {
		// Goroutines is the current number of goroutines
		Goroutines int `json:"goroutines"`
		// HeapAllocBytes is the bytes of allocated heap objects
		HeapAllocBytes uint64 `json:"heap_alloc_bytes"`
		// HeapSysBytes is the bytes of heap memory obtained from the OS
		HeapSysBytes uint64 `json:"heap_sys_bytes"`
		// HeapIdleBytes is the bytes in idle (unused) spans
		HeapIdleBytes uint64 `json:"heap_idle_bytes"`
		// HeapInUseBytes is the bytes in in-use spans
		HeapInUseBytes uint64 `json:"heap_in_use_bytes"`
		// HeapReleasedBytes is the bytes of physical memory returned to the OS
		HeapReleasedBytes uint64 `json:"heap_released_bytes"`
		// HeapObjects is the number of allocated heap objects
		HeapObjects uint64 `json:"heap_objects"`
		// GCCycles is the number of completed GC cycles
		GCCycles uint32 `json:"gc_cycles"`
		// GCPauseTotalNs is the cumulative nanoseconds in GC stop-the-world pauses
		GCPauseTotalNs uint64 `json:"gc_pause_total_ns"`
		// StackInUseBytes is the bytes in stack spans
		StackInUseBytes uint64 `json:"stack_in_use_bytes"`
		// StackSysBytes is the bytes of stack memory obtained from the OS
		StackSysBytes uint64 `json:"stack_sys_bytes"`
	} `json:"runtime"`

	// Errors contains error information from metrics collection
	Errors struct {
		// CPUError contains CPU metrics collection error if any
		CPUError string `json:"cpu_error,omitempty"`
		// MemoryError contains memory metrics collection error if any
		MemoryError string `json:"memory_error,omitempty"`
		// NetworkError contains network metrics collection error if any
		NetworkError string `json:"network_error,omitempty"`
	} `json:"errors"`
}

const (
	// DefaultCacheInterval is the default interval for background metrics collection.
	// The background goroutine collects metrics every 2 seconds to balance freshness and performance.
	DefaultCacheInterval = 2 * time.Second

	// MaxCacheAge is the maximum age before cached values are considered stale.
	// Values older than 10 seconds are considered outdated and trigger cache staleness warnings.
	MaxCacheAge = 10 * time.Second

	// StringFormatBufferSize is the initial capacity for string formatting buffers.
	// Pre-allocated to 32 bytes to handle typical percentage and byte count formatting.
	StringFormatBufferSize = 32

	// ProcessCacheSeconds is the process cache duration in seconds.
	// Process objects are cached for 30 seconds to avoid expensive OS calls.
	ProcessCacheSeconds = 30
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

	// stopChan is used to signal the background collection goroutine to stop
	stopChan chan struct{}
	// running indicates whether background collection is currently active
	running atomic.Bool
}

var (
	cachedMetrics  = &CachedMetrics{}
	metricsMu      sync.Mutex
	stringBuffer   = sync.Pool{New: func() any { return make([]byte, 0, StringFormatBufferSize) }}
	memStatsBuffer = sync.Pool{New: func() any { return &runtime.MemStats{} }}
)

// StartMetricsCollection initializes the background metrics collection system.
// It starts a goroutine that periodically collects CPU, memory, and network metrics
// using atomic operations for lock-free access. This function is idempotent and
// safe to call multiple times.
//
// The background collection runs every DefaultCacheInterval (2 seconds) and provides
// cached metrics that can be accessed with sub-microsecond latency.
func StartMetricsCollection() {
	//nolint:contextcheck // Convenience wrapper; use StartMetricsCollectionWithContext when you have an inherited ctx.
	StartMetricsCollectionWithContext(context.Background())
}

// StartMetricsCollectionWithContext initializes the background metrics collection system
// using ctx as the base context for gopsutil calls (timeouts are still applied per tick).
//
// Note: ctx cancellation may cause subsequent collection cycles to fail fast; prefer
// passing a long-lived application context.
//
//nolint:contextcheck // Background collector is not request-scoped; ctx here is a best-effort base for gopsutil calls.
func StartMetricsCollectionWithContext(ctx context.Context) {
	metricsMu.Lock()
	defer metricsMu.Unlock()

	if cachedMetrics.running.Load() {
		return
	}

	ctx = corecontext.OrBackground(ctx)
	baseCtx := ctx

	stop := make(chan struct{})
	cachedMetrics.stopChan = stop
	cachedMetrics.running.Store(true)

	// Initialize with zero values
	cachedMetrics.ServiceCPU.Store("0.00%")
	cachedMetrics.SystemCPU.Store("0.00%")
	cachedMetrics.MemoryData.Store(&MemoryLoadData{
		Service: "0.00%",
		System:  "0.00%",
		Total:   "0",
	})

	go backgroundMetricsCollection(baseCtx, stop)
}

// StopMetricsCollection stops the background metrics collection system.
// It signals the background goroutine to terminate and stops all metric updates.
// This function is idempotent and safe to call multiple times.
//
// After calling this function, cached metrics will become stale and
// IsCacheStale() will return true after MaxCacheAge duration.
func StopMetricsCollection() {
	metricsMu.Lock()
	if !cachedMetrics.running.Load() {
		metricsMu.Unlock()
		return
	}
	stop := cachedMetrics.stopChan
	cachedMetrics.running.Store(false)
	cachedMetrics.stopChan = nil
	metricsMu.Unlock()

	if stop != nil {
		close(stop)
	}
}

// backgroundMetricsCollection runs metrics collection in the background.
// It collects CPU, memory, and network metrics concurrently at regular intervals.
// This function runs in its own goroutine and should not be called directly.
func backgroundMetricsCollection(baseCtx context.Context, stop <-chan struct{}) {
	ticker := time.NewTicker(DefaultCacheInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Bound the gopsutil calls so we don't risk blocking forever in the background.
			ctx, cancel := corecontext.WithMaxTimeout(baseCtx, DefaultCacheInterval)
			collectMetrics(ctx)
			cancel()
		case <-stop:
			return
		}
	}
}

// collectMetrics collects all metrics concurrently in the background.
// It spawns separate goroutines for CPU, memory, and network collection
// to minimize the total collection time and avoid blocking operations.
func collectMetrics(ctx context.Context) {
	now := time.Now().Unix()

	// Define collection tasks
	tasks := []func(context.Context, int64){
		collectCPUMetrics,
		collectMemoryMetrics,
		collectNetworkMetrics,
	}

	// Run collection tasks concurrently.
	// Since tasks themselves handle internal timeouts and don't return errors,
	// we only check for context cancellation.
	if err := concurrency.Process(ctx, tasks, func(ctx context.Context, task func(context.Context, int64)) error {
		task(ctx, now)
		return nil
	}, concurrency.BatchConfig[func(context.Context, int64)]{}); err != nil {
		// Context cancellation is expected during shutdown.
		if !coreerrs.IsContextCanceled(err) {
			// In case of other errors (though unlikely here), we don't want to panic.
			return
		}
	}
}

// collectCPUMetrics collects CPU metrics with process caching optimizations.
// It caches the process object to avoid expensive OS calls and uses
// atomic operations to store the results for lock-free access.
func collectCPUMetrics(ctx context.Context, timestamp int64) {
	// Use cached process if available and recent
	var proc *process.Process
	if cached := cachedMetrics.ProcessCache.Load(); cached != nil {
		if time.Now().Unix()-cachedMetrics.ProcessTimestamp.Load() < ProcessCacheSeconds {
			proc = cached
		}
	}

	if proc == nil {
		var err error
		proc, err = ProcessDetails(ctx)
		if err != nil {
			return
		}
		cachedMetrics.ProcessCache.Store(proc)
		cachedMetrics.ProcessTimestamp.Store(timestamp)
	}

	// Get service CPU (fast)
	serviceCPU, err := proc.CPUPercent()
	if err == nil {
		if bufInterface := stringBuffer.Get(); bufInterface != nil {
			buf, ok := bufInterface.([]byte)
			if !ok {
				return
			}
			buf = strconv.AppendFloat(buf[:0], serviceCPU, 'f', 2, 64)
			buf = append(buf, '%')
			cachedMetrics.ServiceCPU.Store(string(buf))
			stringBuffer.Put(buf) //nolint:staticcheck
		}
	}

	// Get system CPU (slow - 1 second)
	// Use interval of 0 to get instant reading based on previous call
	cpuPercents, err := cpu.PercentWithContext(ctx, 0, false)
	if err == nil && len(cpuPercents) > 0 {
		if bufInterface := stringBuffer.Get(); bufInterface != nil {
			buf, ok := bufInterface.([]byte)
			if !ok {
				return
			}
			buf = strconv.AppendFloat(buf[:0], cpuPercents[0], 'f', 2, 64)
			buf = append(buf, '%')
			cachedMetrics.SystemCPU.Store(string(buf))
			stringBuffer.Put(buf) //nolint:staticcheck
		}
	}

	cachedMetrics.CPUTimestamp.Store(timestamp)
}

// collectMemoryMetrics collects memory usage statistics for both system and service.
// It retrieves virtual memory statistics and process-specific memory information,
// then formats them as percentages and stores them atomically.
func collectMemoryMetrics(ctx context.Context, timestamp int64) {
	vmStat, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return
	}

	// Get process if available
	proc := cachedMetrics.ProcessCache.Load()
	if proc == nil {
		var processErr error
		proc, processErr = ProcessDetails(ctx)
		if processErr != nil {
			return
		}
	}

	memInfo, err := proc.MemoryInfoWithContext(ctx)
	if err != nil {
		return
	}

	// Pre-allocate buffers for string formatting
	serviceBufInterface := stringBuffer.Get()
	serviceBuf, ok := serviceBufInterface.([]byte)
	if !ok {
		return
	}

	systemBufInterface := stringBuffer.Get()
	systemBuf, ok := systemBufInterface.([]byte)
	if !ok {
		stringBuffer.Put(serviceBuf) //nolint:staticcheck
		return
	}

	totalBufInterface := stringBuffer.Get()
	totalBuf, ok := totalBufInterface.([]byte)
	if !ok {
		stringBuffer.Put(serviceBuf) //nolint:staticcheck
		stringBuffer.Put(systemBuf)  //nolint:staticcheck
		return
	}

	serviceBuf = strconv.AppendFloat(serviceBuf[:0], float64(memInfo.RSS)/float64(vmStat.Total)*100, 'f', 2, 64)
	serviceBuf = append(serviceBuf, '%')

	systemBuf = strconv.AppendFloat(systemBuf[:0], vmStat.UsedPercent, 'f', 2, 64)
	systemBuf = append(systemBuf, '%')

	totalBuf = strconv.AppendFloat(totalBuf[:0], float64(vmStat.Total), 'f', 2, 64)

	memData := &MemoryLoadData{
		Service: string(serviceBuf),
		System:  string(systemBuf),
		Total:   string(totalBuf),
	}

	cachedMetrics.MemoryData.Store(memData)
	cachedMetrics.MemoryTimestamp.Store(timestamp)

	// Return buffers to pool
	stringBuffer.Put(serviceBuf) //nolint:staticcheck
	stringBuffer.Put(systemBuf)  //nolint:staticcheck
	stringBuffer.Put(totalBuf)   //nolint:staticcheck
}

// collectNetworkMetrics collects cumulative network I/O statistics.
// It aggregates bytes sent and received across all network interfaces
// and stores the totals atomically for fast access.
func collectNetworkMetrics(_ context.Context, timestamp int64) {
	netIO, err := net.IOCounters(true)
	if err != nil {
		return
	}

	var totalBytesReceived, totalBytesSent uint64
	for i := range netIO {
		totalBytesReceived += netIO[i].BytesRecv
		totalBytesSent += netIO[i].BytesSent
	}

	cachedMetrics.NetworkReceived.Store(totalBytesReceived)
	cachedMetrics.NetworkSent.Store(totalBytesSent)
	cachedMetrics.NetworkTimestamp.Store(timestamp)
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

// CPULoad returns cached CPU load statistics for both the service and the system.
// It provides sub-microsecond response times by using pre-collected cached data.
// The function automatically starts background metrics collection if not already running.
//
// The context parameter is accepted for API compatibility but is not currently used
// since this function returns cached data immediately.
//
// Returns CPULoadData with Service and System CPU usage as formatted percentages,
// or an error if metrics collection fails (though errors are rare with caching).
func CPULoad(ctx context.Context) (*CPULoadData, error) {
	StartMetricsCollectionWithContext(ctx)

	serviceCP := cachedMetrics.ServiceCPU.Load().(string) //nolint:errcheck
	systemCPU := cachedMetrics.SystemCPU.Load().(string)  //nolint:errcheck

	return &CPULoadData{
		Service: serviceCP,
		System:  systemCPU,
		Total:   systemCPU,
	}, nil
}

// ProcessDetails returns the process details for the current application.
// It creates a new process object using the current process ID and can be used
// to query detailed process information such as CPU usage, memory consumption, etc.
//
// The context can be used to cancel the operation if it takes too long.
//
// Returns a process.Process object for the current application process,
// or an error if the process cannot be found or accessed.
func ProcessDetails(ctx context.Context) (*process.Process, error) {
	pid := ProcessId()
	proc, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return nil, coreerrs.Wrapf(err, "get process details for pid %d", pid)
	}
	return proc, nil
}

// ProcessId returns the process ID of the current application.
// This is a wrapper around os.Getpid() that returns the PID as int32
// for compatibility with the gopsutil library.
//
// Returns the current process ID as int32.
func ProcessId() int32 {
	return int32(os.Getpid()) // #nosec G115 -- PIDs fit in int32 on all supported platforms
}

// MemoryLoad returns cached memory usage statistics for both the service and the system.
// It provides sub-microsecond response times by using pre-collected cached data.
// The function automatically starts background metrics collection if not already running.
//
// The context parameter is accepted for API compatibility but is not currently used
// since this function returns cached data immediately.
//
// Returns MemoryLoadData with Service and System memory usage as formatted percentages
// and Total memory in bytes, or an error if metrics collection fails.
func MemoryLoad(ctx context.Context) (*MemoryLoadData, error) {
	StartMetricsCollectionWithContext(ctx)

	memData := cachedMetrics.MemoryData.Load()
	if memData == nil {
		return &MemoryLoadData{
			Service: "0.00%",
			System:  "0.00%",
			Total:   "0",
		}, nil
	}

	// Return a copy to avoid race conditions
	return &MemoryLoadData{
		Service: memData.Service,
		System:  memData.System,
		Total:   memData.Total,
	}, nil
}

// VirtualMemory returns the current virtual memory statistics for the system.
// This function makes a direct system call and is not cached, unlike MemoryLoad.
// Use this function when you need detailed memory statistics beyond the basic metrics.
//
// The context can be used to cancel the operation if it takes too long.
//
// Returns virtual memory statistics including total, available, used memory and percentages,
// or an error if the system call fails.
func VirtualMemory(ctx context.Context) (*mem.VirtualMemoryStat, error) {
	return mem.VirtualMemoryWithContext(ctx)
}

// NetworkIO returns cached network I/O statistics for the service.
// It provides sub-microsecond response times by using pre-collected cached data.
// The function automatically starts background metrics collection if not already running.
//
// The context parameter is accepted for API compatibility but is not currently used
// since this function returns cached data immediately.
//
// Returns total bytes received and sent since process start as float64 values,
// or an error if metrics collection fails (though errors are rare with caching).
func NetworkIO(ctx context.Context) (float64, float64, error) {
	StartMetricsCollectionWithContext(ctx)

	received := float64(cachedMetrics.NetworkReceived.Load())
	sent := float64(cachedMetrics.NetworkSent.Load())

	return received, sent, nil
}

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

// NumGoroutines returns the current number of goroutines in the application.
// This is a wrapper around runtime.NumGoroutine() for consistency with other stats functions.
//
// Returns the number of goroutines that currently exist in the application.
func NumGoroutines() int {
	return runtime.NumGoroutine()
}

// GetApplicationStats returns comprehensive application statistics including CPU, memory,
// network I/O, and Go runtime metrics. It provides sub-millisecond response times by using
// cached data collected by a background goroutine.
//
// The function automatically starts background metrics collection if not already running.
// All metrics are collected concurrently and cached using atomic operations for lock-free access.
//
// The context parameter is accepted for API compatibility but is not currently used
// for most operations since cached data is returned immediately.
//
// Returns ApplicationStats containing all available system and runtime metrics,
// with error information populated in the Errors field if any collection fails.
func GetApplicationStats(ctx context.Context) *ApplicationStats {
	stats := &ApplicationStats{
		ProcessID: ProcessId(),
		Uptime:    Uptime(),
	}

	// Use cached metrics for fast response
	cpuData, _ := CPULoad(ctx) //nolint:errcheck
	stats.CPU.Service = cpuData.Service
	stats.CPU.System = cpuData.System

	memData, _ := MemoryLoad(ctx) //nolint:errcheck
	stats.Memory.Service = memData.Service
	stats.Memory.System = memData.System
	stats.Memory.Total = memData.Total

	bytesReceived, bytesSent, err := NetworkIO(ctx)
	if err != nil {
		stats.Errors.NetworkError = err.Error()
		stats.Network.BytesReceived = "error"
		stats.Network.BytesSent = "error"
		stats.Network.RawReceived = -1
		stats.Network.RawSent = -1
	} else {
		// Pre-allocate buffer for formatting
		if bufInterface := stringBuffer.Get(); bufInterface != nil {
			buf, ok := bufInterface.([]byte)
			if !ok {
				// Fallback to fmt.Sprintf if type assertion fails
				stats.Network.BytesReceived = fmt.Sprintf("%.0f bytes", bytesReceived)
				stats.Network.BytesSent = fmt.Sprintf("%.0f bytes", bytesSent)
			} else {
				buf = strconv.AppendFloat(buf[:0], bytesReceived, 'f', 0, 64)
				buf = append(buf, " bytes"...)
				stats.Network.BytesReceived = string(buf)

				buf = strconv.AppendFloat(buf[:0], bytesSent, 'f', 0, 64)
				buf = append(buf, " bytes"...)
				stats.Network.BytesSent = string(buf)

				stringBuffer.Put(buf) //nolint:staticcheck
			}
		} else {
			// Fallback if Get() returns nil
			stats.Network.BytesReceived = fmt.Sprintf("%.0f bytes", bytesReceived)
			stats.Network.BytesSent = fmt.Sprintf("%.0f bytes", bytesSent)
		}

		stats.Network.RawReceived = bytesReceived
		stats.Network.RawSent = bytesSent
	}

	// Go runtime statistics (optimized)
	memStats := MemStats()
	stats.Runtime.Goroutines = NumGoroutines()
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

	return stats
}
