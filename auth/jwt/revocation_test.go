// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist"
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

func TestVerifierRevocationEmptyJTIUnchecked(t *testing.T) {
	t.Parallel()
	dl := denylist.New()
	dl.Revoke("") // a blank revocation must never match a token without a jti
	v := jwt.NewVerifier(nil, jwt.WithRevocation(dl))

	require.NoError(t, v.ValidateClaims(jwt.Claims{"exp": futureExp()}))
}

func TestVerifierNoRevocationConfigured(t *testing.T) {
	t.Parallel()
	v := jwt.NewVerifier(nil)
	require.NoError(t, v.ValidateClaims(jwt.Claims{"jti": "tok-1", "exp": futureExp()}))
}
