// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// syncHourly schedules the revocation sync, which turns the staleness bound
// on, without the cron firing during a test.
const syncHourly = "@every 1h"

// backdateRevocationSync moves the provider's last successful revocation sync
// age into the past.
func backdateRevocationSync(p *Provider, age time.Duration) {
	p.lastRevocationSyncUnixNanos.Store(time.Now().Add(-age).UnixNano())
}

// A revocation source that keeps failing must not leave the last snapshot
// answering "not revoked" forever: past the staleness bound the check fails
// closed, and the next successful sync restores it.
func TestRevocationStaleness_FailsClosedUntilNextSync(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	raw := idp.sign(t, idp.claims(nil))
	sourceDown := errors.New("source down")
	storage := &syncStorage{script: []error{nil /* construction */, sourceDown, nil}}
	p := idp.newProvider(t, WithRevocationStorage(storage), WithRevocationSyncSchedule(syncHourly),
		WithRevocationMaxStaleness(time.Minute))

	_, err := p.ValidateToken(t.Context(), raw)
	require.NoError(t, err, "a fresh snapshot is trusted")

	require.ErrorIs(t, p.scheduledRevocationSync(t.Context()), sourceDown)
	backdateRevocationSync(p, 2*time.Minute)

	_, err = p.ValidateToken(t.Context(), raw)
	require.ErrorIs(t, err, ErrRevocationCheck)
	require.ErrorIs(t, err, ErrRevocationStale)

	require.NoError(t, p.scheduledRevocationSync(t.Context()))
	_, err = p.ValidateToken(t.Context(), raw)
	require.NoError(t, err, "a successful sync makes the snapshot fresh again")
}

func TestRevocationStaleness_FailModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    []Option
		wantErr error
	}{
		{name: "no bound keeps trusting the snapshot", opts: []Option{WithRevocationMaxStaleness(0)}},
		{name: "fail-open accepts a stale snapshot", opts: []Option{WithRevocationMaxStaleness(time.Minute), WithRevocationFailOpen()}},
		{name: "bound rejects a stale snapshot", opts: []Option{WithRevocationMaxStaleness(time.Minute)}, wantErr: ErrRevocationStale},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			idp := newTestIdP(t)
			raw := idp.sign(t, idp.claims(nil))
			base := []Option{WithRevocationStorage(&syncStorage{}), WithRevocationSyncSchedule(syncHourly)}
			p := idp.newProvider(t, append(base, tc.opts...)...)
			backdateRevocationSync(p, time.Hour)

			_, err := p.ValidateToken(t.Context(), raw)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

// A revoked answer is still reported as revoked, not as a failed check, when
// the snapshot is stale.
func TestRevocationStaleness_RevokedStaysRevoked(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	raw := idp.sign(t, idp.claims(nil))
	idp.setRevoked(raw)
	p := idp.newProvider(t,
		WithRevocationFilter(newRebuildableFilter()),
		WithRevocationLoader(&URLRevocationLoader{URL: idp.srv.URL + "/revoked"}),
		WithRevocationSyncSchedule(syncHourly),
		WithRevocationMaxStaleness(time.Minute),
	)
	backdateRevocationSync(p, time.Hour)

	_, err := p.ValidateToken(t.Context(), raw)
	require.ErrorIs(t, err, ErrTokenRevoked)
}
