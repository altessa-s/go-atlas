// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist"
	"github.com/altessa-s/go-atlas/auth/jwt"
	"github.com/altessa-s/go-atlas/auth/selfjwt"
)

// TestVerifyRejectsRevoked confirms WithRevocation forwards the denylist to the
// embedded jwt verifier: a freshly minted token verifies until its jti is
// revoked, then fails with jwt.ErrTokenRevoked.
func TestVerifyRejectsRevoked(t *testing.T) {
	t.Parallel()
	p := newProvider(t, jwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, testIssuer, testAudience, selfjwt.WithMinterClock(fixedClock{t: baseTime}))
	dl := denylist.New()
	v := selfjwt.NewVerifier(p, testIssuer, testAudience, selfjwt.WithVerifierClock(fixedClock{t: baseTime}), selfjwt.WithRevocation(dl))

	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	_, err = v.Verify(t.Context(), res.Token)
	require.NoError(t, err) // not revoked yet

	dl.Revoke(res.ID)
	_, err = v.Verify(t.Context(), res.Token)
	require.ErrorIs(t, err, jwt.ErrTokenRevoked)
}

func TestFractionalExpiryRetention(t *testing.T) {
	t.Parallel()
	p := newProvider(t, jwt.AlgEdDSA)
	key, err := p.SigningKey(t.Context(), testSubject)
	require.NoError(t, err)
	now := time.Unix(1750000000, 0).UTC()
	for _, literal := range []string{"1750000001.0000000001", "1.7500000010000000001e9"} {
		raw, err := jwt.NewSigner().Sign(key, jwt.Claims{"sub": testSubject, "iss": testIssuer, "aud": testAudience, "exp": json.Number(literal), "jti": "retained"})
		require.NoError(t, err)
		tok, err := selfjwt.NewVerifier(p, testIssuer, testAudience, selfjwt.WithVerifierClock(fixedClock{t: now})).Verify(t.Context(), raw)
		require.NoError(t, err)
		require.Equal(t, now.Add(time.Second+time.Nanosecond), tok.Expiry)
		lastTick := now.Add(time.Second + jwt.DefaultLeeway)
		dl := denylist.New(denylist.WithClock(func() time.Time { return lastTick }))
		dl.RevokeUntil(tok.ID, tok.Expiry.Add(jwt.DefaultLeeway))
		require.True(t, dl.IsRevoked(tok.ID))
	}
}
