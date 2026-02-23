// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package stats_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/probfilter/stats"
)

func TestFilterStats_JSONRoundTrip(t *testing.T) {
	original := &stats.FilterStats{
		Capacity:          100000,
		ItemCount:         5000,
		FillRatio:         0.05,
		FalsePositiveRate: 0.01,
		MemoryUsageBytes:  1024,
		LastRebuild:       time.Now().Truncate(time.Second),
		StorageType:       "memory",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	var decoded stats.FilterStats
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	if decoded.Capacity != original.Capacity {
		t.Errorf("Capacity = %d, want %d", decoded.Capacity, original.Capacity)
	}
	if decoded.ItemCount != original.ItemCount {
		t.Errorf("ItemCount = %d, want %d", decoded.ItemCount, original.ItemCount)
	}
	if decoded.FillRatio != original.FillRatio {
		t.Errorf("FillRatio = %f, want %f", decoded.FillRatio, original.FillRatio)
	}
	if decoded.FalsePositiveRate != original.FalsePositiveRate {
		t.Errorf("FalsePositiveRate = %f, want %f", decoded.FalsePositiveRate, original.FalsePositiveRate)
	}
	if decoded.MemoryUsageBytes != original.MemoryUsageBytes {
		t.Errorf("MemoryUsageBytes = %d, want %d", decoded.MemoryUsageBytes, original.MemoryUsageBytes)
	}
	if decoded.StorageType != original.StorageType {
		t.Errorf("StorageType = %q, want %q", decoded.StorageType, original.StorageType)
	}
}

func TestFilterStats_JSONOmitEmpty(t *testing.T) {
	fs := &stats.FilterStats{
		Capacity:    1000,
		StorageType: "redis",
	}

	data, err := json.Marshal(fs)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	var m map[string]any
	_ = json.Unmarshal(data, &m)

	if _, ok := m["memoryUsageBytes"]; ok {
		t.Error("memoryUsageBytes should be omitted when zero")
	}
	// Note: time.Time zero value marshals as "0001-01-01T00:00:00Z", not omitted
	// because Go's omitempty considers non-nil structs as non-empty
}

func TestFilterStats_ZeroValues(t *testing.T) {
	fs := &stats.FilterStats{}
	if fs.Capacity != 0 {
		t.Errorf("Capacity = %d, want 0", fs.Capacity)
	}
	if fs.StorageType != "" {
		t.Errorf("StorageType = %q, want empty", fs.StorageType)
	}
}

func TestFilterStats_Fields(t *testing.T) {
	tests := []struct {
		name  string
		stats stats.FilterStats
		check func(t *testing.T, s stats.FilterStats)
	}{
		{
			name:  "full capacity",
			stats: stats.FilterStats{Capacity: 100, ItemCount: 100, FillRatio: 1.0},
			check: func(t *testing.T, s stats.FilterStats) {
				if s.FillRatio != 1.0 {
					t.Errorf("FillRatio = %f, want 1.0", s.FillRatio)
				}
			},
		},
		{
			name:  "empty filter",
			stats: stats.FilterStats{Capacity: 100, ItemCount: 0, FillRatio: 0.0},
			check: func(t *testing.T, s stats.FilterStats) {
				if s.ItemCount != 0 {
					t.Errorf("ItemCount = %d, want 0", s.ItemCount)
				}
			},
		},
		{
			name:  "with memory usage",
			stats: stats.FilterStats{MemoryUsageBytes: 4096},
			check: func(t *testing.T, s stats.FilterStats) {
				if s.MemoryUsageBytes != 4096 {
					t.Errorf("MemoryUsageBytes = %d, want 4096", s.MemoryUsageBytes)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, tt.stats)
		})
	}
}
