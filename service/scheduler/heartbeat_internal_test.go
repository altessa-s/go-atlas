// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestConfirmRunOwnership drives confirmRunOwnership with a scripted clock.
// since answers come from the script in call order; once it runs out, the
// lease reads as fresh. The fake renewal stops the test from looping forever
// if the bound is ever lost.
func TestConfirmRunOwnership(t *testing.T) {
	t.Parallel()
	const interval = time.Second
	stale, fresh := interval, time.Duration(0)
	errRenew := errors.New("renew failed")

	for _, tc := range []struct {
		name         string
		since        []time.Duration
		renewed      bool
		renewErr     error
		wantOwned    bool
		wantRenewals int
	}{
		{name: "fast_claim", since: []time.Duration{fresh}, renewed: true, wantOwned: true},
		{name: "one_fresh_renewal", since: []time.Duration{stale, fresh}, renewed: true, wantOwned: true, wantRenewals: 1},
		{name: "rejected", since: []time.Duration{stale}, renewed: false, wantRenewals: 1},
		{name: "renew_error", since: []time.Duration{stale}, renewed: true, renewErr: errRenew, wantRenewals: 1},
		{
			name:    "late_responses",
			since:   []time.Duration{stale, stale, stale, stale, stale},
			renewed: true, wantRenewals: maxOwnershipConfirmations,
		},
		{
			// The last allowed renewal is fresh when checked, but the loop
			// condition then finds the lease stale again (a pause in between):
			// no further renewal may be issued.
			name:    "pause_after_last_check",
			since:   []time.Duration{stale, stale, stale, fresh, stale, stale, stale, stale, stale, stale},
			renewed: true, wantRenewals: maxOwnershipConfirmations,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			script := tc.since
			since := func(time.Time) time.Duration {
				if len(script) == 0 {
					return fresh
				}
				d := script[0]
				script = script[1:]
				return d
			}
			renewals := 0
			renew := func() (time.Time, bool, error) {
				renewals++
				require.LessOrEqual(t, renewals, 10, "confirmation is unbounded")
				return time.Now(), tc.renewed, tc.renewErr
			}

			_, owned := confirmRunOwnership(renew, since, time.Now(), interval)

			require.Equal(t, tc.wantOwned, owned)
			require.Equal(t, tc.wantRenewals, renewals)
		})
	}
}
