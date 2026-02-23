// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package concurrency

import (
	"runtime"
)

// ConcurrencyLimitFunc is a function that returns the current concurrency limit
// based on runtime conditions such as available memory, CPU count, or system load.
// It is called each time a limit decision is needed, so implementations may adapt
// dynamically. Use the built-in factories [MemoryAwareConcurrency],
// [LoadAwareConcurrency], [AdaptiveConcurrency], or [ConnectionPoolAwareConcurrency]
// to create common implementations.
type ConcurrencyLimitFunc func() int

// Tuning constants for concurrency limit calculations.
const (
	// DefaultConcurrencyMultiplier is applied to runtime.NumCPU() for I/O-bound
	// workloads in [DefaultLimitFunc] and [ConcurrencyForEnvironment].
	DefaultConcurrencyMultiplier = 2

	// MinConcurrencyLimit is the absolute minimum returned by any built-in
	// [ConcurrencyLimitFunc] implementation.
	MinConcurrencyLimit = 1

	// MaxConcurrencyLimit is the upper cap applied by [AdaptiveConcurrency]
	// to prevent runaway goroutine creation.
	MaxConcurrencyLimit = 1000

	// HighThroughputMultiplier is the CPU multiplier used for [EnvironmentHighThroughput].
	HighThroughputMultiplier = 4

	// RateLimitedConcurrency is the fixed concurrency used for [EnvironmentRateLimited]
	// to avoid exceeding external rate limits.
	RateLimitedConcurrency = 3

	// ConcurrencyDivider halves the CPU count, used by [LoadAwareConcurrency]
	// under medium system load.
	ConcurrencyDivider = 2

	// BytesToMB is the divisor used twice (bytes -> KB -> MB) when converting
	// memory statistics to megabytes.
	BytesToMB = 1024

	// Pressure scaling factors for concurrency adjustment
	severePressureFactor = 0.25 // Factor for severe resource pressure
	mediumPressureFactor = 0.5  // Factor for medium resource pressure
)

// Environment identifies the workload profile for [ConcurrencyForEnvironment].
type Environment string

const (
	// EnvironmentMemoryConstrained selects a single worker to minimize heap usage.
	EnvironmentMemoryConstrained Environment = "memory-constrained"

	// EnvironmentCPUBound matches concurrency to runtime.NumCPU(), suitable for
	// compute-heavy workloads where more goroutines would cause contention.
	EnvironmentCPUBound Environment = "cpu-bound"

	// EnvironmentIOBound doubles the CPU count, which is the default recommendation
	// for workloads that spend most of their time waiting on I/O.
	EnvironmentIOBound Environment = "io-bound"

	// EnvironmentHighThroughput quadruples the CPU count for maximum parallelism
	// in throughput-oriented workloads with low per-item overhead.
	EnvironmentHighThroughput Environment = "high-throughput"

	// EnvironmentRateLimited uses a fixed small concurrency (3) to avoid
	// exceeding external API rate limits.
	EnvironmentRateLimited Environment = "rate-limited"
)

// ConcurrencyForEnvironment returns the recommended concurrency limit for a given environment type.
// This is a helper function that provides sensible defaults for common deployment scenarios.
//
// Environment types:
//   - memory-constrained: 1 worker (minimal memory usage)
//   - cpu-bound: runtime.NumCPU() workers (match CPU cores)
//   - io-bound: runtime.NumCPU() * 2 workers (default recommendation)
//   - high-throughput: runtime.NumCPU() * 4 workers (maximum performance)
//   - rate-limited: 3 workers (conservative to avoid rate limits)
//
// Returns the recommended concurrency limit for the specified environment.
func ConcurrencyForEnvironment(env Environment) int {
	cpuCount := runtime.NumCPU()

	switch env {
	case EnvironmentMemoryConstrained:
		return MinConcurrencyLimit // Single worker to minimize memory usage
	case EnvironmentCPUBound:
		return cpuCount // Match CPU cores for CPU-bound tasks
	case EnvironmentIOBound:
		return cpuCount * DefaultConcurrencyMultiplier // Default I/O bound recommendation
	case EnvironmentHighThroughput:
		return cpuCount * HighThroughputMultiplier // Aggressive parallelism for high throughput
	case EnvironmentRateLimited:
		return RateLimitedConcurrency // Conservative limit to avoid rate limiting
	default:
		return cpuCount * DefaultConcurrencyMultiplier // Safe default
	}
}

// DefaultLimitFunc returns the default concurrency limit, which is the
// recommendation for [EnvironmentIOBound] (runtime.NumCPU() * 2). It is used
// as the fallback when neither [BatchConfig.LimitFunc] nor a positive
// [BatchConfig.Concurrency] value is provided.
func DefaultLimitFunc() int {
	return ConcurrencyForEnvironment(EnvironmentIOBound)
}

// MemoryAwareConcurrency creates a concurrency limit function that adjusts based on available memory.
// It reads runtime memory statistics and adjusts concurrency based on memory pressure.
//
// Memory stats are cached with a TTL (default 1 second) to avoid frequent
// runtime.ReadMemStats calls which cause stop-the-world pauses.
// Use SetMemStatsCacheTTL to adjust the cache duration if needed.
//
// Parameters:
//   - lowMemoryMB: memory threshold (in MB) below which concurrency is set to 1
//   - mediumMemoryMB: memory threshold (in MB) below which concurrency is conservative
//   - highMemoryMB: memory threshold (in MB) above which concurrency is aggressive
//
// Returns a function that can be used with WithConcurrencyLimitFunc options.
func MemoryAwareConcurrency(lowMemoryMB, mediumMemoryMB, highMemoryMB uint64) ConcurrencyLimitFunc {
	return func() int {
		// Use cached memory stats to avoid frequent STW pauses
		availableMemoryMB := getAvailableMemoryMB()

		cpuCount := runtime.NumCPU()

		switch {
		case availableMemoryMB < lowMemoryMB:
			return MinConcurrencyLimit // Minimal concurrency for low memory
		case availableMemoryMB < mediumMemoryMB:
			return cpuCount // Conservative concurrency for medium memory
		case availableMemoryMB < highMemoryMB:
			return cpuCount * DefaultConcurrencyMultiplier // Default concurrency for good memory
		default:
			return cpuCount * HighThroughputMultiplier // Aggressive concurrency for high memory
		}
	}
}

// LoadAwareConcurrency creates a concurrency limit function that adjusts based on system load.
// It takes a function that returns the current system load and adjusts concurrency accordingly.
//
// Parameters:
//   - getLoad: function that returns current system load (typically from /proc/loadavg)
//   - highLoadThreshold: load threshold above which concurrency is minimized
//   - mediumLoadThreshold: load threshold above which concurrency is reduced
//
// Returns a function that can be used with WithConcurrencyLimitFunc options.
func LoadAwareConcurrency(getLoad func() float64, highLoadThreshold, mediumLoadThreshold float64) ConcurrencyLimitFunc {
	return func() int {
		load := getLoad()
		cpuCount := runtime.NumCPU()

		switch {
		case load > highLoadThreshold:
			return MinConcurrencyLimit // Minimal concurrency for high load
		case load > mediumLoadThreshold:
			return max(MinConcurrencyLimit, cpuCount/ConcurrencyDivider) // Reduced concurrency for medium load
		case load > mediumLoadThreshold/2:
			return cpuCount // Normal concurrency for low-medium load
		default:
			return cpuCount * HighThroughputMultiplier // High concurrency for low load
		}
	}
}

// AdaptiveConcurrency creates a concurrency limit function that considers multiple factors.
// It combines memory usage, time of day, and system load to determine optimal concurrency.
//
// Memory stats are cached with a TTL (default 1 second) to avoid frequent
// runtime.ReadMemStats calls which cause stop-the-world pauses.
// Use SetMemStatsCacheTTL to adjust the cache duration if needed.
//
// Parameters:
//   - config: configuration for adaptive concurrency behavior
//
// Returns a function that can be used with WithConcurrencyLimitFunc options.
func AdaptiveConcurrency(config AdaptiveConcurrencyConfig) ConcurrencyLimitFunc {
	return func() int {
		cpuCount := runtime.NumCPU()
		baseConcurrency := cpuCount * DefaultConcurrencyMultiplier

		// Start with base concurrency
		concurrency := float64(baseConcurrency)

		// Apply memory pressure penalty using cached stats
		if config.MemoryLowThresholdMB > 0 {
			availableMemoryMB := getAvailableMemoryMB()

			if availableMemoryMB < config.MemoryLowThresholdMB {
				concurrency *= severePressureFactor // Severe memory pressure
			} else if availableMemoryMB < config.MemoryMediumThresholdMB {
				concurrency *= mediumPressureFactor // Medium memory pressure
			}
		}

		// Apply load-based adjustment
		if config.GetSystemLoad != nil && config.HighLoadThreshold > 0 {
			load := config.GetSystemLoad()
			if load > config.HighLoadThreshold {
				concurrency *= severePressureFactor // Severe load pressure
			} else if load > config.HighLoadThreshold/2 {
				concurrency *= mediumPressureFactor // Medium load pressure
			}
		}

		// Ensure minimum concurrency
		result := min(max(int(concurrency), MinConcurrencyLimit), MaxConcurrencyLimit)

		return result
	}
}

// AdaptiveConcurrencyConfig holds the tuning parameters for [AdaptiveConcurrency].
type AdaptiveConcurrencyConfig struct {
	// MemoryLowThresholdMB is the available-memory threshold (in MB) below which
	// concurrency is reduced to 25% of the base value (severe pressure).
	MemoryLowThresholdMB uint64

	// MemoryMediumThresholdMB is the available-memory threshold (in MB) below which
	// concurrency is reduced to 50% of the base value (medium pressure).
	MemoryMediumThresholdMB uint64

	// GetSystemLoad returns the current system load average. When non-nil and
	// HighLoadThreshold > 0, the returned value is compared against
	// HighLoadThreshold to apply additional concurrency scaling.
	GetSystemLoad func() float64

	// HighLoadThreshold is the system load above which concurrency is scaled
	// down. Values above this threshold apply a 25% factor; values above half
	// this threshold apply a 50% factor.
	HighLoadThreshold float64
}

// ConnectionPoolAwareConcurrency creates a concurrency limit function that considers available connections.
// It adjusts concurrency based on available connections in the connection pool.
//
// Parameters:
//   - getAvailableConnections: function that returns number of available connections
//   - reservedConnections: number of connections to keep in reserve
//
// Returns a function that can be used with WithConcurrencyLimitFunc options.
func ConnectionPoolAwareConcurrency(getAvailableConnections func() int, reservedConnections int) ConcurrencyLimitFunc {
	return func() int {
		availableConns := getAvailableConnections()
		cpuBasedLimit := runtime.NumCPU() * DefaultConcurrencyMultiplier

		// Use available connections minus reserved, but don't exceed CPU-based limit
		connectionBasedLimit := max(MinConcurrencyLimit, availableConns-reservedConnections)

		return min(cpuBasedLimit, connectionBasedLimit)
	}
}
