// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt_test

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist"
	"github.com/altessa-s/go-atlas/auth/denylist/mirror"
	"github.com/altessa-s/go-atlas/auth/jwt"
)

// futureExp is an exp claim an hour out, satisfying the verifier's
// expiration-required default so the revocation check is reached.
func futureExp() float64 { return float64(time.Now().Add(time.Hour).Unix()) }

func TestVerifierRevocationRejectsRevokedJTI(t *testing.T) {
	t.Parallel()
	dl := denylist.New()
	v := jwt.NewVerifier(nil, jwt.WithRevocation(dl))
	claims := jwt.Claims{"jti": "tok-1", "exp": futureExp()}

	require.NoError(t, v.ValidateClaims(claims)) // not revoked yet

	dl.Revoke("tok-1")
	require.ErrorIs(t, v.ValidateClaims(claims), jwt.ErrTokenRevoked)

	dl.Restore("tok-1")
	require.NoError(t, v.ValidateClaims(claims)) // restored
}

func TestVerifierRevocationRequiresJTI(t *testing.T) {
	t.Parallel()
	v := jwt.NewVerifier(nil, jwt.WithRevocation(denylist.New()))

	err := v.ValidateClaims(jwt.Claims{"exp": futureExp()})
	require.ErrorIs(t, err, jwt.ErrClaimMissing)

	err = v.ValidateClaims(jwt.Claims{"jti": float64(7), "exp": futureExp()})
	require.ErrorIs(t, err, jwt.ErrClaimMissing, "a non-string jti is not an identifier")
}

func TestVerifierRevocationAllowMissingID(t *testing.T) {
	t.Parallel()
	dl := denylist.New()
	dl.Revoke("") // a blank revocation must never match a token without a jti
	v := jwt.NewVerifier(nil, jwt.WithRevocation(dl), jwt.WithInsecureRevocationAllowMissingID())

	require.NoError(t, v.ValidateClaims(jwt.Claims{"exp": futureExp()}))
}

func TestVerifierUnprimedMirrorRejects(t *testing.T) {
	t.Parallel()
	m := mirror.New(emptySource{})
	v := jwt.NewVerifier(nil, jwt.WithRevocation(m))
	claims := jwt.Claims{"jti": "tok-1", "exp": futureExp()}

	err := v.ValidateClaims(claims)
	require.ErrorIs(t, err, jwt.ErrRevocationUnavailable)
	require.NotErrorIs(t, err, jwt.ErrTokenRevoked)

	require.NoError(t, m.Prime(t.Context()))
	require.NoError(t, v.ValidateClaims(claims))
}

func TestVerifierStaleMirrorRejects(t *testing.T) {
	t.Parallel()
	src := &flakySource{}
	now := time.Unix(1_750_000_000, 0)
	m := mirror.New(src, mirror.WithMaxStaleness(time.Minute), mirror.WithClock(func() time.Time { return now }))
	require.NoError(t, m.Prime(t.Context()))
	v := jwt.NewVerifier(nil, jwt.WithRevocation(m))
	claims := jwt.Claims{"jti": "tok-1", "exp": futureExp()}
	require.NoError(t, v.ValidateClaims(claims))

	src.fail.Store(true)
	now = now.Add(time.Hour)
	require.Error(t, m.Refresh(t.Context()))
	require.ErrorIs(t, v.ValidateClaims(claims), jwt.ErrRevocationUnavailable)
}

type emptySource struct{}

func (emptySource) StreamValues(context.Context) iter.Seq2[string, error] {
	return func(func(string, error) bool) {}
}

type flakySource struct{ fail atomic.Bool }

func (s *flakySource) StreamValues(context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		if s.fail.Load() {
			yield("", errors.New("source down"))
		}
	}
}

func TestVerifierNoRevocationConfigured(t *testing.T) {
	t.Parallel()
	v := jwt.NewVerifier(nil)
	require.NoError(t, v.ValidateClaims(jwt.Claims{"jti": "tok-1", "exp": futureExp()}))
}

type failingChecker struct{ err error }

func (c failingChecker) IsRevoked(string) (bool, error) { return false, c.err }

// A checker error never reaches the caller's error text: the classification
// is kept, the dependency's own message is not.
func TestVerifierRevocationErrorHidesCheckerText(t *testing.T) {
	t.Parallel()
	for name, cause := range map[string]error{
		"plain":   errors.New("MARKER-secret-dsn"),
		"wrapped": fmt.Errorf("%w: MARKER-secret-dsn", jwt.ErrRevocationUnavailable),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			v := jwt.NewVerifier(nil, jwt.WithRevocation(failingChecker{err: cause}))
			err := v.ValidateClaims(jwt.Claims{"jti": "tok-1", "exp": futureExp()})
			require.ErrorIs(t, err, jwt.ErrRevocationUnavailable)
			require.NotContains(t, err.Error(), "MARKER")
		})
	}
}
