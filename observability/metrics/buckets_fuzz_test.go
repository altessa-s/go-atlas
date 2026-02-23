// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"math"
	"testing"
)

func FuzzLinearBuckets(f *testing.F) {
	f.Add(0.0, 10.0, 5)
	f.Add(1.0, 0.5, 10)
	f.Add(-1.0, 1.0, 3)

	f.Fuzz(func(t *testing.T, start, width float64, count int) {
		if math.IsNaN(start) || math.IsNaN(width) || math.IsInf(start, 0) || math.IsInf(width, 0) {
			return
		}
		if count > 1000 {
			count = 1000
		}
		result := LinearBuckets(start, width, count)
		if count <= 0 && result != nil {
			t.Error("expected nil for non-positive count")
		}
		if count > 0 && len(result) != count {
			t.Errorf("expected %d buckets, got %d", count, len(result))
		}
	})
}

func FuzzExponentialBuckets(f *testing.F) {
	f.Add(1.0, 2.0, 5)
	f.Add(0.1, 10.0, 3)

	f.Fuzz(func(t *testing.T, start, factor float64, count int) {
		if math.IsNaN(start) || math.IsNaN(factor) || math.IsInf(start, 0) || math.IsInf(factor, 0) {
			return
		}
		if count > 1000 {
			count = 1000
		}
		result := ExponentialBuckets(start, factor, count)
		if (count <= 0 || start <= 0 || factor <= 1) && result != nil {
			t.Error("expected nil for invalid params")
		}
	})
}
