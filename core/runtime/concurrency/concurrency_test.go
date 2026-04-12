// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package concurrency_test

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
)

func TestConcurrencyForEnvironment(t *testing.T) {
	cpu := runtime.NumCPU()

	tests := []struct {
		env  concurrency.Environment
		want int
	}{
		{concurrency.EnvironmentMemoryConstrained, 1},
		{concurrency.EnvironmentCPUBound, cpu},
		{concurrency.EnvironmentIOBound, cpu * 2},
		{concurrency.EnvironmentHighThroughput, cpu * 4},
		{concurrency.EnvironmentRateLimited, 3},
		{"unknown", cpu * 2}, // Default
	}

	for _, tt := range tests {
		t.Run(string(tt.env), func(t *testing.T) {
			require.Equal(t, tt.want, concurrency.ConcurrencyForEnvironment(tt.env))
		})
	}
}

func TestMemoryAwareConcurrency(t *testing.T) {
	// We can't easily mock getAvailableMemoryMB (private const/func) without internal access or re-architecting.
	// For now, we verify that the returned function returns a sensible value (>= 1).
	fn := concurrency.MemoryAwareConcurrency(100, 500, 1000)
	limit := fn()
	require.GreaterOrEqual(t, limit, 1)
}

func TestLoadAwareConcurrency(t *testing.T) {
	// Load function is injectable
	tests := []struct {
		name string
		load float64
		want int // We check against the logic relative to CPU count
	}{
		{"HighLoad", 10.0, 1},
		{"MediumLoad", 2.0, max(1, runtime.NumCPU()/2)},
		{"LowLoad", 0.1, runtime.NumCPU() * 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := concurrency.LoadAwareConcurrency(func() float64 { return tt.load }, 5.0, 1.0)
			require.Equal(t, tt.want, fn())
		})
	}
}

func TestAdaptiveConcurrency(t *testing.T) {
	cpu := runtime.NumCPU()
	base := cpu * 2 // Default multiplier

	config := concurrency.AdaptiveConcurrencyConfig{
		GetSystemLoad:     func() float64 { return 0.0 }, // No load
		HighLoadThreshold: 10.0,
		// Memory thresholds ignored since we can't mock private memory check easily here
		// But passing 0 disables memory check in AdaptiveConcurrency implementation?
		// "if config.MemoryLowThresholdMB > 0"
		MemoryLowThresholdMB: 0,
	}

	fn := concurrency.AdaptiveConcurrency(config)
	require.Equal(t, base, fn())

	// Test with high load
	config.GetSystemLoad = func() float64 { return 20.0 }
	fn = concurrency.AdaptiveConcurrency(config)
	// High load -> severe pressure -> * 0.25
	want := max(1, int(float64(base)*0.25))

	require.Equal(t, want, fn())
}

func TestConnectionPoolAwareConcurrency(t *testing.T) {
	cpu := runtime.NumCPU()
	defaultMax := cpu * 2

	t.Run("PlentyConnections", func(t *testing.T) {
		fn := concurrency.ConnectionPoolAwareConcurrency(func() int { return defaultMax + 100 }, 0)
		require.Equal(t, defaultMax, fn())
	})

	t.Run("LimitedConnections", func(t *testing.T) {
		limit := 5
		fn := concurrency.ConnectionPoolAwareConcurrency(func() int { return limit }, 0)
		require.Equal(t, limit, fn())
	})

	t.Run("ReservedConnections", func(t *testing.T) {
		avail := 10
		reserved := 4
		fn := concurrency.ConnectionPoolAwareConcurrency(func() int { return avail }, reserved)
		require.Equal(t, avail-reserved, fn())
	})
}
