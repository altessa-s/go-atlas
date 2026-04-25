// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sampler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAlwaysOn(t *testing.T) {
	s := AlwaysOn()
	result := s.ShouldSample(SamplingParameters{})
	require.Equal(t, RecordAndSample, result.Decision)
	require.Equal(t, "AlwaysOnSampler", s.Description())
}

func TestAlwaysOn_Singleton(t *testing.T) {
	require.Same(t, AlwaysOn(), AlwaysOn(), "AlwaysOn should return singleton")
}

func TestAlwaysOff(t *testing.T) {
	s := AlwaysOff()
	result := s.ShouldSample(SamplingParameters{})
	require.Equal(t, Drop, result.Decision)
	require.Equal(t, "AlwaysOffSampler", s.Description())
}

func TestAlwaysOff_Singleton(t *testing.T) {
	require.Same(t, AlwaysOff(), AlwaysOff(), "AlwaysOff should return singleton")
}
