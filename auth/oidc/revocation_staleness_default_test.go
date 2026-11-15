// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
)

// The staleness bound is on by default, but only for a provider that syncs its
// revocation storage on a schedule; zero or a negative value opts out.
func TestRevocationStaleness_DefaultBound(t *testing.T) {
	t.Parallel()

	require.GreaterOrEqual(t, DefaultRevocationMaxStaleness, 15*time.Minute)

	scheduled := func(opts ...Option) []Option {
		return append([]Option{WithRevocationSyncSchedule(syncHourly)}, opts...)
	}
	tests := []struct {
		name    string
		opts    []Option
		age     time.Duration
		wantErr error
	}{
		{name: "scheduled sync: past the default bound", opts: scheduled(), age: DefaultRevocationMaxStaleness + time.Minute, wantErr: ErrRevocationStale},
		{name: "scheduled sync: within the default bound", opts: scheduled(), age: DefaultRevocationMaxStaleness - time.Minute},
		{name: "no scheduled sync: nothing can be stale", age: 10 * DefaultRevocationMaxStaleness},
		{name: "explicit bound without a scheduled sync", opts: []Option{WithRevocationMaxStaleness(time.Minute)}, age: time.Hour},
		{name: "zero opts out", opts: scheduled(WithRevocationMaxStaleness(0)), age: 10 * DefaultRevocationMaxStaleness},
		{name: "negative opts out", opts: scheduled(WithRevocationMaxStaleness(-time.Second)), age: 10 * DefaultRevocationMaxStaleness},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			idp := newTestIdP(t)
			raw := idp.sign(t, idp.claims(nil))
			p := idp.newProvider(t, append([]Option{WithRevocationStorage(&syncStorage{})}, tc.opts...)...)
			backdateRevocationSync(p, tc.age)

			_, err := p.ValidateToken(t.Context(), raw)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, ErrRevocationCheck)
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

// A fail-open provider that starts on a failed initial sync keeps its startup
// behavior: tokens are accepted without a failed check, and the bound counts
// from construction so the scheduled sync has time to catch up.
func TestRevocationStaleness_FailOpenStartupCountsFromConstruction(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	raw := idp.sign(t, idp.claims(nil))
	collector := metricstest.NewTestCollector()
	p := idp.newProvider(t, WithMeter(collector),
		WithRevocationStorage(&syncStorage{err: errors.New("source down")}),
		WithRevocationSyncSchedule(syncHourly), WithRevocationFailOpen())

	_, err := p.ValidateToken(t.Context(), raw)
	require.NoError(t, err)
	require.Equal(t, 1.0, metricstest.GetCounterValue(t, collector, revocationErrorsMetric),
		"only the failed initial sync is counted")

	backdateRevocationSync(p, DefaultRevocationMaxStaleness+time.Minute)
	_, err = p.ValidateToken(t.Context(), raw)
	require.NoError(t, err, "fail-open accepts a stale snapshot")
	require.Equal(t, 2.0, metricstest.GetCounterValue(t, collector, revocationErrorsMetric))
}
