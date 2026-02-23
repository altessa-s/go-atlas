// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"math"
	"slices"
	"testing"
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
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBuckets() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMustValidateBuckets_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustValidateBuckets should panic on invalid buckets")
		}
	}()
	MustValidateBuckets([]float64{2.0, 1.0}, "test")
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
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if math.Abs(got[i]-tt.want[i]) > 1e-10 {
					t.Errorf("bucket[%d] = %f, want %f", i, got[i], tt.want[i])
				}
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
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("ExponentialBuckets() = %v, want %v", got, tt.want)
			}
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
			if !slices.Equal(got, tt.want) {
				t.Errorf("MergeBuckets() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCopyBuckets(t *testing.T) {
	orig := []float64{1, 2, 3}
	cp := CopyBuckets(orig)
	cp[0] = 99
	if orig[0] == 99 {
		t.Error("CopyBuckets should create independent copy")
	}
}

func TestDefaultBuckets_StrictlyIncreasing(t *testing.T) {
	for _, buckets := range [][]float64{DefaultDurationBuckets, DefaultSizeBuckets, DefaultQuantileBuckets} {
		if err := ValidateBuckets(buckets, "default"); err != nil {
			t.Errorf("default buckets invalid: %v", err)
		}
	}
}
