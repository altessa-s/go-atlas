// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sampler

import "testing"

func TestParentBased_NoParent_UsesRoot(t *testing.T) {
	s := NewParentBased(AlwaysOn())
	result := s.ShouldSample(SamplingParameters{})
	if result.Decision != RecordAndSample {
		t.Errorf("expected RecordAndSample, got %v", result.Decision)
	}
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
	if result.Decision != RecordAndSample {
		t.Errorf("expected RecordAndSample for remote sampled parent, got %v", result.Decision)
	}
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
	if result.Decision != Drop {
		t.Errorf("expected Drop for remote not-sampled parent, got %v", result.Decision)
	}
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
	if result.Decision != RecordAndSample {
		t.Errorf("expected RecordAndSample for local sampled parent, got %v", result.Decision)
	}
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
	if result.Decision != Drop {
		t.Errorf("expected Drop for local not-sampled parent, got %v", result.Decision)
	}
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
	if result.Decision != Drop {
		t.Errorf("expected Drop with custom remote sampled, got %v", result.Decision)
	}
}

func TestParentBased_InvalidParent_UsesRoot(t *testing.T) {
	s := NewParentBased(AlwaysOn())
	result := s.ShouldSample(SamplingParameters{
		ParentCtx: &SpanContext{}, // zero = invalid
	})
	if result.Decision != RecordAndSample {
		t.Errorf("invalid parent should fall through to root, got %v", result.Decision)
	}
}

func TestParentBased_Description(t *testing.T) {
	s := NewParentBased(AlwaysOn())
	want := "ParentBased{root:AlwaysOnSampler}"
	if s.Description() != want {
		t.Errorf("Description = %q, want %q", s.Description(), want)
	}
}
