// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sampler

import "testing"

func TestAlwaysOn(t *testing.T) {
	s := AlwaysOn()
	result := s.ShouldSample(SamplingParameters{})

	if result.Decision != RecordAndSample {
		t.Errorf("expected RecordAndSample, got %v", result.Decision)
	}
	if s.Description() != "AlwaysOnSampler" {
		t.Errorf("Description = %q", s.Description())
	}
}

func TestAlwaysOn_Singleton(t *testing.T) {
	if AlwaysOn() != AlwaysOn() {
		t.Error("AlwaysOn should return singleton")
	}
}

func TestAlwaysOff(t *testing.T) {
	s := AlwaysOff()
	result := s.ShouldSample(SamplingParameters{})

	if result.Decision != Drop {
		t.Errorf("expected Drop, got %v", result.Decision)
	}
	if s.Description() != "AlwaysOffSampler" {
		t.Errorf("Description = %q", s.Description())
	}
}

func TestAlwaysOff_Singleton(t *testing.T) {
	if AlwaysOff() != AlwaysOff() {
		t.Error("AlwaysOff should return singleton")
	}
}
