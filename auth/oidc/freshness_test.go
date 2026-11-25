// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"bytes"
	"context"
	"log/slog"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"golang.org/x/oauth2"
)

// Record the elapsed clock reading at each distinct wall instant. Looking up
// the original time.Time also detects stripping its monotonic component.
type rollbackClock struct {
	wall    time.Time
	ticks   time.Duration
	samples map[time.Time]time.Duration
}

func (c *rollbackClock) observe() time.Time {
	c.samples[c.wall] = c.ticks
	return c.wall
}
func (c *rollbackClock) elapsed(at time.Time) time.Duration {
	if tick, ok := c.samples[at]; ok {
		return c.ticks - tick
	}
	return c.wall.Sub(at)
}

func TestFreshnessSurvivesWallRollback(t *testing.T) {
	t.Parallel()
	for _, policy := range []string{"JWKS", "revocation"} {
		t.Run(policy, func(t *testing.T) {
			t.Parallel()
			p := newStalenessTestProvider(t, time.Minute, JWKSFailureModeEnforce)
			c := &rollbackClock{wall: time.Now(), samples: make(map[time.Time]time.Duration)}
			p.revocation.clock = freshnessClock{now: c.observe, since: c.elapsed}
			p.keys.clock = p.revocation.clock
			p.revocation.storage = &syncStorage{}

			p.revocation.syncSchedule = syncHourly
			p.revocation.maxStaleness = time.Minute
			p.keys.markJWKSRefreshed()
			p.revocation.markRevocationSynced()
			check := p.revocation.checkRevocationStaleness
			want := ErrRevocationStale
			if policy == "JWKS" {
				check = func() error { return p.keys.checkJWKSStaleness(t.Context()) }
				want = ErrJWKSStale
			}
			require.NoError(t, check())
			c.ticks = 2 * time.Minute
			c.wall = c.wall.Add(2 * time.Minute)
			require.ErrorIs(t, check(), want)
			c.wall = c.wall.Add(-90 * time.Second)
			require.ErrorIs(t, check(), want)
		})
	}
}

type peerAgeStorage struct {
	syncStorage
	read func() (time.Time, time.Duration, bool, error)
}

func (s *peerAgeStorage) LastCommittedSync(context.Context) (time.Time, time.Duration, bool, error) {
	return s.read()
}

func TestPeerFreshnessConsumesRPCDelay(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name               string
		age, delay, budget time.Duration
	}{
		{"delayed", time.Second, time.Minute + time.Nanosecond, time.Minute},
		{"negative", -1, 0, time.Minute},
		{"saturated", math.MaxInt64, 1, math.MaxInt64},
		{"near overflow", math.MaxInt64 - 1, 2, math.MaxInt64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := &rollbackClock{wall: time.Now(), samples: make(map[time.Time]time.Duration)}
			p := newStalenessTestProvider(t, 0, JWKSFailureModeDisabled)
			p.revocation.clock = freshnessClock{now: c.observe, since: c.elapsed}
			p.keys.clock = p.revocation.clock

			p.revocation.syncSchedule = syncHourly
			p.revocation.maxStaleness = tc.budget
			if tc.name == "delayed" {
				p.revocation.markRevocationSynced()
			} // local sync expires in flight
			p.revocation.storage = &peerAgeStorage{read: func() (time.Time, time.Duration, bool, error) {
				c.ticks += tc.delay
				c.wall = c.wall.Add(tc.delay)
				return time.Unix(123, 0), tc.age, true, nil
			}}
			p.revocation.refreshSharedRevocationSync(t.Context())
			require.ErrorIs(t, p.revocation.checkRevocationStaleness(), ErrRevocationStale)
			if tc.age < 0 || tc.age == math.MaxInt64 {
				require.Nil(t, p.revocation.sharedRevocationSync.Load())
			}
		})
	}
}

func TestSignedUserInfoFreshness(t *testing.T) {
	t.Parallel()
	for _, mode := range []JWKSFailureMode{JWKSFailureModeEnforce, JWKSFailureModeWarn} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			idp := newTestIdP(t)
			var logs bytes.Buffer
			p := idp.newProvider(t, WithJWKSMaxStaleness(time.Minute), WithJWKSFailureMode(mode), WithLogger(slog.New(slog.NewTextHandler(&logs, nil))))
			p.discovery.info.UserInfoURL = idp.srv.URL + "/userinfo"
			signed := idp.sign(t, idp.claims(nil))
			original := p.discovery.client.Transport
			p.discovery.client.Transport = testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/userinfo" {
					return original.RoundTrip(r)
				}
				resp := jsonResponse(r, signed)
				resp.Header.Set("Content-Type", "application/jwt")
				return resp, nil
			})
			setStaleness(t, p, 2*time.Minute)
			token := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test"})
			info, err := p.UserInfo(t.Context(), token)
			if mode == JWKSFailureModeEnforce {
				require.ErrorIs(t, err, ErrJWKSStale)
			} else {
				require.NoError(t, err)
				require.Equal(t, "user-1", info.Id)
				require.Contains(t, logs.String(), "JWKSFailureModeWarn")
			}
			require.NoError(t, p.RefreshJWKS(t.Context()))
			info, err = p.UserInfo(t.Context(), token)
			require.NoError(t, err)
			require.Equal(t, "user-1", info.Id)
		})
	}
}

func TestPeerClockRollbackCannotRenewRejectedCommit(t *testing.T) {
	t.Parallel()
	const budget = time.Minute
	committed := time.Now()
	age := budget + time.Second
	p := newStalenessTestProvider(t, 0, JWKSFailureModeDisabled)

	p.revocation.syncSchedule = syncHourly
	p.revocation.maxStaleness = budget
	p.revocation.storage = &peerAgeStorage{read: func() (time.Time, time.Duration, bool, error) { return committed, age, true, nil }}
	p.revocation.refreshSharedRevocationSync(t.Context())
	require.ErrorIs(t, p.revocation.checkRevocationStaleness(), ErrRevocationStale)
	age = time.Second
	p.revocation.refreshSharedRevocationSync(t.Context())
	require.ErrorIs(t, p.revocation.checkRevocationStaleness(), ErrRevocationStale)
	committed = committed.Add(time.Minute)
	p.revocation.refreshSharedRevocationSync(t.Context())
	require.NoError(t, p.revocation.checkRevocationStaleness())
}
