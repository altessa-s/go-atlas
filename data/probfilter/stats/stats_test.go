// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package stats_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
	require.NoError(t, err)

	var decoded stats.FilterStats
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, original.Capacity, decoded.Capacity)
	require.Equal(t, original.ItemCount, decoded.ItemCount)
	require.Equal(t, original.FillRatio, decoded.FillRatio)
	require.Equal(t, original.FalsePositiveRate, decoded.FalsePositiveRate)
	require.Equal(t, original.MemoryUsageBytes, decoded.MemoryUsageBytes)
	require.Equal(t, original.StorageType, decoded.StorageType)
}

func TestFilterStats_JSONOmitEmpty(t *testing.T) {
	fs := &stats.FilterStats{
		Capacity:    1000,
		StorageType: "redis",
	}

	data, err := json.Marshal(fs)
	require.NoError(t, err)

	var m map[string]any
	_ = json.Unmarshal(data, &m)

	_, hasMemUsage := m["memoryUsageBytes"]
	require.False(t, hasMemUsage, "memoryUsageBytes should be omitted when zero")
	// Note: time.Time zero value marshals as "0001-01-01T00:00:00Z", not omitted
	// because Go's omitempty considers non-nil structs as non-empty
}

func TestFilterStats_ZeroValues(t *testing.T) {
	fs := &stats.FilterStats{}
	require.Equal(t, int64(0), fs.Capacity)
	require.Equal(t, "", fs.StorageType)
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
				require.Equal(t, 1.0, s.FillRatio)
			},
		},
		{
			name:  "empty filter",
			stats: stats.FilterStats{Capacity: 100, ItemCount: 0, FillRatio: 0.0},
			check: func(t *testing.T, s stats.FilterStats) {
				require.Equal(t, int64(0), s.ItemCount)
			},
		},
		{
			name:  "with memory usage",
			stats: stats.FilterStats{MemoryUsageBytes: 4096},
			check: func(t *testing.T, s stats.FilterStats) {
				require.Equal(t, int64(4096), s.MemoryUsageBytes)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, tt.stats)
		})
	}
}
