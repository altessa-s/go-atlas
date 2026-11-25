// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCPUCollectorPublishesIndependently(t *testing.T) {
	t.Parallel()
	var cache CachedMetrics
	cache.updateCPU(t.Context(), 42)
	require.NotNil(t, cache.ProcessCache.Load())
	require.EqualValues(t, 42, cache.CPUTimestamp.Load())
	for _, value := range []any{cache.ServiceCPU.Load(), cache.SystemCPU.Load()} {
		require.IsType(t, "", value)
		s, _ := value.(string)
		require.True(t, strings.HasSuffix(s, "%"))
		_, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		require.NoError(t, err)
	}
	require.Nil(t, cache.MemoryData.Load())
}

func TestMemoryCollectorPublishesIndependently(t *testing.T) {
	t.Parallel()
	var cache CachedMetrics
	cache.publishMemory(collectMemory(t.Context(), cache.ProcessCache.Load), 43)
	data := cache.MemoryData.Load()
	require.NotNil(t, data)
	total, err := strconv.ParseFloat(data.Total, 64)
	require.NoError(t, err)
	require.Positive(t, total)
	require.True(t, strings.HasSuffix(data.Service, "%"))
	require.True(t, strings.HasSuffix(data.System, "%"))
	require.EqualValues(t, 43, cache.MemoryTimestamp.Load())
	require.Nil(t, cache.ProcessCache.Load())
}

func TestFailedMemoryReadKeepsLastSample(t *testing.T) {
	t.Parallel()
	var cache CachedMetrics
	previous := &MemoryLoadData{Service: "1.00%", System: "2.00%", Total: "1024.00"}
	cache.MemoryData.Store(previous)
	cache.MemoryTimestamp.Store(42)
	cache.publishMemory(nil, 43)
	require.Same(t, previous, cache.MemoryData.Load())
	require.EqualValues(t, 42, cache.MemoryTimestamp.Load())
}

func TestNetworkCollectorPublishesIndependently(t *testing.T) {
	t.Parallel()
	var cache CachedMetrics
	cache.updateNetwork(t.Context(), 44)
	require.EqualValues(t, 44, cache.NetworkTimestamp.Load())
	require.Zero(t, cache.CPUTimestamp.Load())
	require.Zero(t, cache.MemoryTimestamp.Load())
}

func TestRuntimeCollectorSamplesGoroutinesAndGC(t *testing.T) {
	t.Parallel()
	before := MemStats()
	var stats ApplicationStats
	collectRuntime(&stats)
	after := MemStats()
	require.Positive(t, stats.Runtime.Goroutines)
	require.Positive(t, stats.Runtime.HeapAllocBytes)
	require.GreaterOrEqual(t, stats.Runtime.GCCycles, before.NumGC)
	require.LessOrEqual(t, stats.Runtime.GCCycles, after.NumGC)
	require.GreaterOrEqual(t, stats.Runtime.GCPauseTotalNs, before.PauseTotalNs)
	require.LessOrEqual(t, stats.Runtime.GCPauseTotalNs, after.PauseTotalNs)
}
