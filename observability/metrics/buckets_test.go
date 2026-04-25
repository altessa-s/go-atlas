// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateBuckets(t *testing.T) {
	tests := []struct {
		name    string
		buckets []float64
		wantErr bool
	}{
		{"valid increasing", []float64{0.1, 0.5, 1.0, 5.0}, false},
		{"empty", []float64{}, true},
		{"single", []float64{1.0}, false},
		{"not increasing", []float64{1.0, 0.5, 2.0}, true},
		{"duplicate", []float64{1.0, 1.0, 2.0}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBuckets(tt.buckets, "test")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMustValidateBuckets_Panics(t *testing.T) {
	require.Panics(t, func() {
		MustValidateBuckets([]float64{2.0, 1.0}, "test")
	})
}

func TestLinearBuckets(t *testing.T) {
	tests := []struct {
		name         string
		start, width float64
		count        int
		want         []float64
	}{
		{"basic", 0, 10, 5, []float64{0, 10, 20, 30, 40}},
		{"fractional", 0.1, 0.1, 3, []float64{0.1, 0.2, 0.3}},
		{"zero count", 0, 10, 0, nil},
		{"negative count", 0, 10, -1, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LinearBuckets(tt.start, tt.width, tt.count)
			if tt.want == nil {
				require.Nil(t, got)
				return
			}
			require.Len(t, got, len(tt.want))
			for i := range got {
				require.InDelta(t, tt.want[i], got[i], 1e-10, "bucket[%d]", i)
			}
		})
	}
}

func TestExponentialBuckets(t *testing.T) {
	tests := []struct {
		name          string
		start, factor float64
		count         int
		want          []float64
	}{
		{"basic", 1, 2, 5, []float64{1, 2, 4, 8, 16}},
		{"zero count", 1, 2, 0, nil},
		{"factor <= 1", 1, 1, 5, nil},
		{"start <= 0", 0, 2, 5, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExponentialBuckets(tt.start, tt.factor, tt.count)
			if tt.want == nil {
				require.Nil(t, got)
				return
			}
			require.True(t, slices.Equal(got, tt.want), "ExponentialBuckets() = %v, want %v", got, tt.want)
		})
	}
}

func TestMergeBuckets(t *testing.T) {
	tests := []struct {
		name string
		sets [][]float64
		want []float64
	}{
		{"empty", nil, nil},
		{"single set", [][]float64{{1, 2, 3}}, []float64{1, 2, 3}},
		{"merge with dedup", [][]float64{{1, 3, 5}, {2, 3, 4}}, []float64{1, 2, 3, 4, 5}},
		{"all empty", [][]float64{{}, {}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeBuckets(tt.sets...)
			require.True(t, slices.Equal(got, tt.want), "MergeBuckets() = %v, want %v", got, tt.want)
		})
	}
}

func TestCopyBuckets(t *testing.T) {
	orig := []float64{1, 2, 3}
	cp := CopyBuckets(orig)
	cp[0] = 99
	require.NotEqual(t, float64(99), orig[0], "CopyBuckets should create independent copy")
}

func TestDefaultBuckets_StrictlyIncreasing(t *testing.T) {
	for _, buckets := range [][]float64{DefaultDurationBuckets, DefaultSizeBuckets, DefaultQuantileBuckets} {
		require.NoError(t, ValidateBuckets(buckets, "default"))
	}
}
