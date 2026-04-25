// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProcessId(t *testing.T) {
	pid := ProcessId()
	require.Equal(t, int32(os.Getpid()), pid)
}

func TestNumGoroutines(t *testing.T) {
	n := NumGoroutines()
	require.GreaterOrEqual(t, n, 1, "NumGoroutines() should be >= 1")
}

func TestMemStats(t *testing.T) {
	stats := MemStats()
	require.NotNil(t, stats)
	require.NotZero(t, stats.Alloc, "MemStats().Alloc should be > 0")
}

func TestUptime(t *testing.T) {
	u := Uptime()
	require.Greater(t, u, time.Duration(0), "Uptime() should be > 0")
}

func TestCPULoad(t *testing.T) {
	StartMetricsCollection()
	defer StopMetricsCollection()

	time.Sleep(100 * time.Millisecond)

	data, err := CPULoad(t.Context())
	require.NoError(t, err)
	require.NotNil(t, data)
}

func TestMemoryLoad(t *testing.T) {
	StartMetricsCollection()
	defer StopMetricsCollection()

	time.Sleep(100 * time.Millisecond)

	data, err := MemoryLoad(t.Context())
	require.NoError(t, err)
	require.NotNil(t, data)
}

func TestNetworkIO(t *testing.T) {
	StartMetricsCollection()
	defer StopMetricsCollection()

	time.Sleep(100 * time.Millisecond)

	_, _, err := NetworkIO(t.Context())
	require.NoError(t, err)
}

func TestGetApplicationStats(t *testing.T) {
	StartMetricsCollection()
	defer StopMetricsCollection()

	time.Sleep(100 * time.Millisecond)

	stats := GetApplicationStats(t.Context())
	require.NotNil(t, stats)
	require.Equal(t, int32(os.Getpid()), stats.ProcessID)
	require.GreaterOrEqual(t, stats.Runtime.Goroutines, 1, "Goroutines should be >= 1")
}

func TestIsCacheStale(t *testing.T) {
	// Before metrics collection, cache should be stale
	StopMetricsCollection()
	_ = IsCacheStale() // just ensure no panic
}

func TestProcessDetails(t *testing.T) {
	p, err := ProcessDetails(t.Context())
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestVirtualMemory(t *testing.T) {
	vm, err := VirtualMemory(t.Context())
	require.NoError(t, err)
	require.NotNil(t, vm)
	require.NotZero(t, vm.Total, "VirtualMemory().Total should be > 0")
}

func TestNewStatsLogger(t *testing.T) {
	logger := NewStatsLogger()
	require.NotNil(t, logger)
}

func TestNewStatsLogger_WithLogger(t *testing.T) {
	l := slog.Default()
	logger := NewStatsLogger(WithLogger(l))
	require.NotNil(t, logger)
}

func TestRunLogCycle(t *testing.T) {
	StartMetricsCollection()
	defer StopMetricsCollection()

	time.Sleep(100 * time.Millisecond)

	logger := NewStatsLogger()
	err := logger.RunLogCycle(t.Context())
	require.NoError(t, err)
}

func TestStartMetricsCollectionWithContext(t *testing.T) {
	StopMetricsCollection()

	ctx := t.Context()
	StartMetricsCollectionWithContext(ctx)
	defer StopMetricsCollection()

	metricsMu.Lock()
	running := cachedMetrics.running.Load()
	metricsMu.Unlock()

	require.True(t, running, "expected running=true after StartMetricsCollectionWithContext")
}
