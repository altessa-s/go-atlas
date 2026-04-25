// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetricsCollection_StartStopRestart(t *testing.T) {
	StopMetricsCollection() // idempotent

	StartMetricsCollection()
	metricsMu.Lock()
	firstStop := cachedMetrics.stopChan
	firstRunning := cachedMetrics.running.Load()
	metricsMu.Unlock()

	require.True(t, firstRunning, "running=false, want true after StartMetricsCollection")
	require.NotNil(t, firstStop, "stopChan=nil, want non-nil after StartMetricsCollection")

	StopMetricsCollection()
	metricsMu.Lock()
	afterStopChan := cachedMetrics.stopChan
	afterStopRunning := cachedMetrics.running.Load()
	metricsMu.Unlock()

	require.False(t, afterStopRunning, "running=true, want false after StopMetricsCollection")
	require.Nil(t, afterStopChan, "stopChan should be nil after StopMetricsCollection")

	StartMetricsCollection()
	metricsMu.Lock()
	secondStop := cachedMetrics.stopChan
	secondRunning := cachedMetrics.running.Load()
	metricsMu.Unlock()

	require.True(t, secondRunning, "running=false, want true after restart StartMetricsCollection")
	require.NotNil(t, secondStop, "stopChan=nil, want non-nil after restart StartMetricsCollection")
	require.False(t, firstStop == secondStop, "stopChan reused across restart, want a fresh channel")

	StopMetricsCollection()
}
