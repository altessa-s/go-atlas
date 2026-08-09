// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRenewIntervalFor_FailureBudget pins the property the renewal ratio exists
// to provide: how many renewal attempts fall inside one lease lifetime. That
// count is the budget for surviving a transient NATS failure without losing
// leadership, and at the previous 0.75 default it was exactly one.
func TestRenewIntervalFor_FailureBudget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		ratio         float64
		wantAttempts  int
		wantInterval  time.Duration
		wantTolerance time.Duration
	}{
		{
			name:          "default absorbs one failure",
			ratio:         DefaultRenewRatio,
			wantAttempts:  3,
			wantInterval:  10 * time.Second / 3,
			wantTolerance: time.Millisecond,
		},
		{
			name:          "three quarters leaves no retry",
			ratio:         0.75,
			wantAttempts:  1,
			wantInterval:  7500 * time.Millisecond,
			wantTolerance: time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := renewIntervalFor(DefaultBucketKeysTTL, tt.ratio)
			require.InDelta(t, tt.wantInterval, got, float64(tt.wantTolerance))

			attempts := int(DefaultBucketKeysTTL / got)
			require.Equal(t, tt.wantAttempts, attempts,
				"renewal attempts inside one %v lease lifetime", DefaultBucketKeysTTL)
		})
	}
}

// TestRenewIntervalFor_DefaultBeatsExpiry pins that the default schedules a
// renewal strictly before the key expires server-side, with enough room left
// for a second attempt.
func TestRenewIntervalFor_DefaultBeatsExpiry(t *testing.T) {
	t.Parallel()

	interval := renewIntervalFor(DefaultBucketKeysTTL, DefaultRenewRatio)

	require.Less(t, 2*interval, DefaultBucketKeysTTL,
		"a second renewal attempt must still land before the lease expires")
}

// TestRenewIntervalFor_ClampsInvalidRatio pins that no configured ratio can
// produce a non-positive tick. time.NewTicker panics on one, and the camping
// goroutine's recover would swallow that panic — leaving an elector that
// reports itself running while never electing anyone.
func TestRenewIntervalFor_ClampsInvalidRatio(t *testing.T) {
	t.Parallel()

	for _, ratio := range []float64{0, -1, 1.5, math.NaN(), math.Inf(1)} {
		got := renewIntervalFor(DefaultBucketKeysTTL, ratio)

		require.Positive(t, got, "ratio %v produced a non-positive renew interval", ratio)
		require.NotPanics(t, func() { time.NewTicker(got).Stop() },
			"ratio %v produced an interval time.NewTicker rejects", ratio)
	}
}

// TestRenewIntervalFor_BoundedByBucketTTL pins that an election TTL larger than
// the server-side key TTL does not stretch the renewal interval past the point
// the key would have expired.
func TestRenewIntervalFor_BoundedByBucketTTL(t *testing.T) {
	t.Parallel()

	got := renewIntervalFor(DefaultBucketKeysTTL+time.Hour, DefaultRenewRatio)

	require.Equal(t, renewIntervalFor(DefaultBucketKeysTTL, DefaultRenewRatio), got,
		"the renewal interval must clamp to the bucket key TTL")
}
