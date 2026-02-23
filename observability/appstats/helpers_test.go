// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats

import "testing"

func TestMetricsCollection_StartStopRestart(t *testing.T) {
	StopMetricsCollection() // idempotent

	StartMetricsCollection()
	metricsMu.Lock()
	firstStop := cachedMetrics.stopChan
	firstRunning := cachedMetrics.running.Load()
	metricsMu.Unlock()

	if !firstRunning {
		t.Fatalf("running=false, want true after StartMetricsCollection")
	}
	if firstStop == nil {
		t.Fatalf("stopChan=nil, want non-nil after StartMetricsCollection")
	}

	StopMetricsCollection()
	metricsMu.Lock()
	afterStopChan := cachedMetrics.stopChan
	afterStopRunning := cachedMetrics.running.Load()
	metricsMu.Unlock()

	if afterStopRunning {
		t.Fatalf("running=true, want false after StopMetricsCollection")
	}
	if afterStopChan != nil {
		t.Fatalf("stopChan=%v, want nil after StopMetricsCollection", afterStopChan)
	}

	StartMetricsCollection()
	metricsMu.Lock()
	secondStop := cachedMetrics.stopChan
	secondRunning := cachedMetrics.running.Load()
	metricsMu.Unlock()

	if !secondRunning {
		t.Fatalf("running=false, want true after restart StartMetricsCollection")
	}
	if secondStop == nil {
		t.Fatalf("stopChan=nil, want non-nil after restart StartMetricsCollection")
	}
	if secondStop == firstStop {
		t.Fatalf("stopChan reused across restart, want a fresh channel")
	}

	StopMetricsCollection()
}
