// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestProcessId(t *testing.T) {
	pid := ProcessId()
	if pid != int32(os.Getpid()) {
		t.Errorf("ProcessId() = %d, want %d", pid, os.Getpid())
	}
}

func TestNumGoroutines(t *testing.T) {
	n := NumGoroutines()
	if n < 1 {
		t.Errorf("NumGoroutines() = %d, want >= 1", n)
	}
}

func TestMemStats(t *testing.T) {
	stats := MemStats()
	if stats == nil {
		t.Fatal("MemStats() returned nil")
	}
	if stats.Alloc == 0 {
		t.Error("MemStats().Alloc should be > 0")
	}
}

func TestUptime(t *testing.T) {
	u := Uptime()
	if u <= 0 {
		t.Errorf("Uptime() = %v, want > 0", u)
	}
}

func TestCPULoad(t *testing.T) {
	StartMetricsCollection()
	defer StopMetricsCollection()

	time.Sleep(100 * time.Millisecond)

	data, err := CPULoad(t.Context())
	if err != nil {
		t.Fatalf("CPULoad() error = %v", err)
	}
	if data == nil {
		t.Fatal("CPULoad() returned nil")
	}
}

func TestMemoryLoad(t *testing.T) {
	StartMetricsCollection()
	defer StopMetricsCollection()

	time.Sleep(100 * time.Millisecond)

	data, err := MemoryLoad(t.Context())
	if err != nil {
		t.Fatalf("MemoryLoad() error = %v", err)
	}
	if data == nil {
		t.Fatal("MemoryLoad() returned nil")
	}
}

func TestNetworkIO(t *testing.T) {
	StartMetricsCollection()
	defer StopMetricsCollection()

	time.Sleep(100 * time.Millisecond)

	_, _, err := NetworkIO(t.Context())
	if err != nil {
		t.Fatalf("NetworkIO() error = %v", err)
	}
}

func TestGetApplicationStats(t *testing.T) {
	StartMetricsCollection()
	defer StopMetricsCollection()

	time.Sleep(100 * time.Millisecond)

	stats := GetApplicationStats(t.Context())
	if stats == nil {
		t.Fatal("GetApplicationStats() returned nil")
	}
	if stats.ProcessID != int32(os.Getpid()) {
		t.Errorf("ProcessID = %d, want %d", stats.ProcessID, os.Getpid())
	}
	if stats.Runtime.Goroutines < 1 {
		t.Error("Goroutines should be >= 1")
	}
}

func TestIsCacheStale(t *testing.T) {
	// Before metrics collection, cache should be stale
	StopMetricsCollection()
	_ = IsCacheStale() // just ensure no panic
}

func TestProcessDetails(t *testing.T) {
	p, err := ProcessDetails(t.Context())
	if err != nil {
		t.Fatalf("ProcessDetails() error = %v", err)
	}
	if p == nil {
		t.Fatal("ProcessDetails() returned nil")
	}
}

func TestVirtualMemory(t *testing.T) {
	vm, err := VirtualMemory(t.Context())
	if err != nil {
		t.Fatalf("VirtualMemory() error = %v", err)
	}
	if vm == nil {
		t.Fatal("VirtualMemory() returned nil")
	}
	if vm.Total == 0 {
		t.Error("VirtualMemory().Total should be > 0")
	}
}

func TestNewStatsLogger(t *testing.T) {
	logger := NewStatsLogger()
	if logger == nil {
		t.Fatal("NewStatsLogger() returned nil")
	}
}

func TestNewStatsLogger_WithLogger(t *testing.T) {
	l := slog.Default()
	logger := NewStatsLogger(WithLogger(l))
	if logger == nil {
		t.Fatal("NewStatsLogger(WithLogger) returned nil")
	}
}

func TestRunLogCycle(t *testing.T) {
	StartMetricsCollection()
	defer StopMetricsCollection()

	time.Sleep(100 * time.Millisecond)

	logger := NewStatsLogger()
	err := logger.RunLogCycle(t.Context())
	if err != nil {
		t.Errorf("RunLogCycle() error = %v", err)
	}
}

func TestStartMetricsCollectionWithContext(t *testing.T) {
	StopMetricsCollection()

	ctx := t.Context()
	StartMetricsCollectionWithContext(ctx)
	defer StopMetricsCollection()

	metricsMu.Lock()
	running := cachedMetrics.running.Load()
	metricsMu.Unlock()

	if !running {
		t.Error("expected running=true after StartMetricsCollectionWithContext")
	}
}
