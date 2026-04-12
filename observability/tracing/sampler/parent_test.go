// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sampler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParentBased_NoParent_UsesRoot(t *testing.T) {
	s := NewParentBased(AlwaysOn())
	result := s.ShouldSample(SamplingParameters{})
	require.Equal(t, RecordAndSample, result.Decision)
}

func TestParentBased_RemoteSampledParent(t *testing.T) {
	s := NewParentBased(AlwaysOff())
	result := s.ShouldSample(SamplingParameters{
		ParentCtx: &SpanContext{
			TraceID:   [16]byte{1},
			SpanID:    [8]byte{1},
			IsSampled: true,
			IsRemote:  true,
		},
	})
	require.Equal(t, RecordAndSample, result.Decision, "expected RecordAndSample for remote sampled parent")
}

func TestParentBased_RemoteNotSampledParent(t *testing.T) {
	s := NewParentBased(AlwaysOn())
	result := s.ShouldSample(SamplingParameters{
		ParentCtx: &SpanContext{
			TraceID:   [16]byte{1},
			SpanID:    [8]byte{1},
			IsSampled: false,
			IsRemote:  true,
		},
	})
	require.Equal(t, Drop, result.Decision, "expected Drop for remote not-sampled parent")
}

func TestParentBased_LocalSampledParent(t *testing.T) {
	s := NewParentBased(AlwaysOff())
	result := s.ShouldSample(SamplingParameters{
		ParentCtx: &SpanContext{
			TraceID:   [16]byte{1},
			SpanID:    [8]byte{1},
			IsSampled: true,
			IsRemote:  false,
		},
	})
	require.Equal(t, RecordAndSample, result.Decision, "expected RecordAndSample for local sampled parent")
}

func TestParentBased_LocalNotSampledParent(t *testing.T) {
	s := NewParentBased(AlwaysOn())
	result := s.ShouldSample(SamplingParameters{
		ParentCtx: &SpanContext{
			TraceID:   [16]byte{1},
			SpanID:    [8]byte{1},
			IsSampled: false,
			IsRemote:  false,
		},
	})
	require.Equal(t, Drop, result.Decision, "expected Drop for local not-sampled parent")
}

func TestParentBased_CustomOptions(t *testing.T) {
	s := NewParentBased(
		AlwaysOff(),
		WithRemoteParentSampled(AlwaysOff()),
		WithRemoteParentNotSampled(AlwaysOn()),
		WithLocalParentSampled(AlwaysOff()),
		WithLocalParentNotSampled(AlwaysOn()),
	)

	// Remote sampled should now Drop (custom override)
	result := s.ShouldSample(SamplingParameters{
		ParentCtx: &SpanContext{
			TraceID:   [16]byte{1},
			SpanID:    [8]byte{1},
			IsSampled: true,
			IsRemote:  true,
		},
	})
	require.Equal(t, Drop, result.Decision, "expected Drop with custom remote sampled")
}

func TestParentBased_InvalidParent_UsesRoot(t *testing.T) {
	s := NewParentBased(AlwaysOn())
	result := s.ShouldSample(SamplingParameters{
		ParentCtx: &SpanContext{}, // zero = invalid
	})
	require.Equal(t, RecordAndSample, result.Decision, "invalid parent should fall through to root")
}

func TestParentBased_Description(t *testing.T) {
	s := NewParentBased(AlwaysOn())
	require.Equal(t, "ParentBased{root:AlwaysOnSampler}", s.Description())
}
