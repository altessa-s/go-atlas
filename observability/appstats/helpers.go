// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats

import (
	"context"
	"fmt"
	"strconv"
	"time"
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

// zeroPercent is the formatted load reported before the first collection.
const zeroPercent = "0.00%"

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

// CPULoad returns cached CPU load statistics for both the service and the system.
// It provides sub-microsecond response times by using pre-collected cached data.
// The function automatically starts background metrics collection if not already running.
//
// The context starts background collection when it is not already running.
//
// Returns CPULoadData with Service and System CPU usage as formatted percentages,
// or an error if metrics collection fails (though errors are rare with caching).
func CPULoad(ctx context.Context) (*CPULoadData, error) {
	if !sampler.running.Load() {
		// The collector outlives the request that happened to start it.
		StartMetricsCollectionWithContext(context.WithoutCancel(ctx))
	}

	serviceCP := cachedMetrics.ServiceCPU.Load().(string) //nolint:errcheck
	systemCPU := cachedMetrics.SystemCPU.Load().(string)  //nolint:errcheck

	return &CPULoadData{
		Service: serviceCP,
		System:  systemCPU,
		Total:   systemCPU,
	}, nil
}

// MemoryLoad returns cached memory usage statistics for both the service and the system.
// It provides sub-microsecond response times by using pre-collected cached data.
// The function automatically starts background metrics collection if not already running.
//
// The context starts background collection when it is not already running.
//
// Returns MemoryLoadData with Service and System memory usage as formatted percentages
// and Total memory in bytes, or an error if metrics collection fails.
func MemoryLoad(ctx context.Context) (*MemoryLoadData, error) {
	if !sampler.running.Load() {
		// The collector outlives the request that happened to start it.
		StartMetricsCollectionWithContext(context.WithoutCancel(ctx))
	}

	memData := cachedMetrics.MemoryData.Load()
	if memData == nil {
		return &MemoryLoadData{
			Service: zeroPercent,
			System:  zeroPercent,
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

// NetworkIO returns cached network I/O statistics for the service.
// It provides sub-microsecond response times by using pre-collected cached data.
// The function automatically starts background metrics collection if not already running.
//
// The context starts background collection when it is not already running.
//
// Returns total bytes received and sent since process start as float64 values,
// or an error if metrics collection fails (though errors are rare with caching).
func NetworkIO(ctx context.Context) (float64, float64, error) {
	if !sampler.running.Load() {
		// The collector outlives the request that happened to start it.
		StartMetricsCollectionWithContext(context.WithoutCancel(ctx))
	}

	received := float64(cachedMetrics.NetworkReceived.Load())
	sent := float64(cachedMetrics.NetworkSent.Load())

	return received, sent, nil
}

// GetApplicationStats returns comprehensive application statistics including CPU, memory,
// network I/O, and Go runtime metrics. It provides sub-millisecond response times by using
// cached data collected by a background goroutine.
//
// The function automatically starts background metrics collection if not already running.
// All metrics are collected concurrently and cached using atomic operations for lock-free access.
//
// The context starts background collection when it is not already running.
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
			bufPtr, ok := bufInterface.(*[]byte)
			if !ok {
				// Fallback to fmt.Sprintf if type assertion fails
				stats.Network.BytesReceived = fmt.Sprintf("%.0f bytes", bytesReceived)
				stats.Network.BytesSent = fmt.Sprintf("%.0f bytes", bytesSent)
			} else {
				buf := strconv.AppendFloat((*bufPtr)[:0], bytesReceived, 'f', 0, 64)
				buf = append(buf, " bytes"...)
				stats.Network.BytesReceived = string(buf)

				buf = strconv.AppendFloat(buf[:0], bytesSent, 'f', 0, 64)
				buf = append(buf, " bytes"...)
				stats.Network.BytesSent = string(buf)

				*bufPtr = buf
				stringBuffer.Put(bufPtr)
			}
		} else {
			// Fallback if Get() returns nil
			stats.Network.BytesReceived = fmt.Sprintf("%.0f bytes", bytesReceived)
			stats.Network.BytesSent = fmt.Sprintf("%.0f bytes", bytesSent)
		}

		stats.Network.RawReceived = bytesReceived
		stats.Network.RawSent = bytesSent
	}

	collectRuntime(stats)
	return stats
}
