// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observe

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithMetricsSubsystem(t *testing.T) {
	t.Parallel()

	sub := "kfocus"
	tests := []struct {
		name string
		opt  Option
		want string
	}{
		{name: "string", opt: WithMetricsSubsystem("egrul"), want: "egrul"},
		{name: "string pointer", opt: WithMetricsSubsystem(&sub), want: "kfocus"},
		{name: "nil pointer keeps default", opt: WithMetricsSubsystem[*string](nil), want: DefaultMetricsSubsystem},
		{name: "empty keeps default", opt: WithMetricsSubsystem(""), want: DefaultMetricsSubsystem},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, newOptions(tc.opt).metricsSubsystem)
		})
	}
}

func TestDefaultOptions(t *testing.T) {
	t.Parallel()

	o := newOptions()
	require.Equal(t, DefaultMetricsSubsystem, o.metricsSubsystem)
	require.Equal(t, DefaultHealthServiceName, o.healthServiceName)
	require.False(t, o.healthPerHost)
	require.Equal(t, DefaultHealthRetryWindow, o.healthRetryWindow)
	require.Equal(t, DefaultHealthRetryBucketCount, o.healthRetryBuckets)
	require.InDelta(t, DefaultHealthRetryDegradedThreshold, o.healthRetryThreshold, 1e-9)
	require.Equal(t, uint64(DefaultHealthRetryMinSamples), o.healthRetryMinSamples)

	require.True(t, newOptions(WithPerHostHealthChecks()).healthPerHost)
}
