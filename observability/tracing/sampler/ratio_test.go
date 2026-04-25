// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sampler

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewTraceIDRatio(t *testing.T) {
	tests := []struct {
		name  string
		ratio float64
		desc  string
	}{
		{"zero returns AlwaysOff", 0.0, "AlwaysOffSampler"},
		{"one returns AlwaysOn", 1.0, "AlwaysOnSampler"},
		{"negative clamped to zero", -0.5, "AlwaysOffSampler"},
		{"above one clamped", 1.5, "AlwaysOnSampler"},
		{"half", 0.5, "TraceIDRatioBased{0.500000}"},
		{"tenth", 0.1, "TraceIDRatioBased{0.100000}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewTraceIDRatio(tt.ratio)
			require.Equal(t, tt.desc, s.Description())
		})
	}
}

func TestTraceIDRatio_Deterministic(t *testing.T) {
	s := NewTraceIDRatio(0.5)
	traceID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8}

	params := SamplingParameters{TraceID: traceID}
	first := s.ShouldSample(params)
	second := s.ShouldSample(params)

	require.Equal(t, first.Decision, second.Decision, "same trace ID should produce same decision")
}

func TestTraceIDRatio_Distribution(t *testing.T) {
	s := NewTraceIDRatio(0.5)
	sampled := 0
	total := 10000

	// Use a large step to spread values across the uint64 range
	step := uint64(math.MaxUint64) / uint64(total)
	for i := range total {
		var traceID [16]byte
		binary.BigEndian.PutUint64(traceID[:8], uint64(i)*step)
		result := s.ShouldSample(SamplingParameters{TraceID: traceID})
		if result.Decision == RecordAndSample {
			sampled++
		}
	}

	ratio := float64(sampled) / float64(total)
	require.True(t, ratio >= 0.4 && ratio <= 0.6, "expected ~50%% sampled, got %.2f%%", ratio*100)
}
