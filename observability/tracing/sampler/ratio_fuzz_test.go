// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sampler

import (
	"math"
	"testing"
)

func FuzzNewTraceIDRatio(f *testing.F) {
	f.Add(0.0)
	f.Add(0.5)
	f.Add(1.0)
	f.Add(-1.0)
	f.Add(2.0)

	f.Fuzz(func(t *testing.T, ratio float64) {
		if math.IsNaN(ratio) || math.IsInf(ratio, 0) {
			return
		}
		s := NewTraceIDRatio(ratio)
		if s == nil {
			t.Fatal("NewTraceIDRatio returned nil")
		}
		if s.Description() == "" {
			t.Error("empty description")
		}

		// Should not panic
		result := s.ShouldSample(SamplingParameters{
			TraceID: [16]byte{1, 2, 3, 4, 5, 6, 7, 8},
		})
		if result.Decision != Drop && result.Decision != RecordAndSample {
			t.Errorf("unexpected decision: %v", result.Decision)
		}
	})
}
