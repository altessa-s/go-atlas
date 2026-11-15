// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/selfjwt"

	atlasjwt "github.com/altessa-s/go-atlas/auth/jwt"
)

func TestMintSetsIssuerAndAudience(t *testing.T) {
	t.Parallel()
	p := newProvider(t, atlasjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, testIssuer, testAudience, clockOpt(baseTime))

	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	claims := jwt.MapClaims{}
	_, _, err = jwt.NewParser().ParseUnverified(res.Token, claims)
	require.NoError(t, err)
	require.Equal(t, testIssuer, claims["iss"])
	aud, err := claims.GetAudience()
	require.NoError(t, err)
	require.Equal(t, jwt.ClaimStrings{testAudience}, aud)
}

// TestVerifyRejectsForeignIssuerOrAudience pins that two services sharing a
// key provider cannot replay each other's tokens.
func TestVerifyRejectsForeignIssuerOrAudience(t *testing.T) {
	t.Parallel()
	p := newProvider(t, atlasjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, testIssuer, testAudience, clockOpt(baseTime))
	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	tests := []struct {
		name, issuer, audience string
	}{
		{name: "other audience", issuer: testIssuer, audience: "other-svc"},
		{name: "other issuer", issuer: "other-issuer", audience: testAudience},
	}
	for _, tc := range tests {
		// Serial: the subtests share p, whose call counter is not
		// concurrency-safe.
		t.Run(tc.name, func(t *testing.T) {
			v := selfjwt.NewVerifier(p, tc.issuer, tc.audience, clockOpt(baseTime))
			_, err := v.Verify(t.Context(), res.Token)
			require.ErrorIs(t, err, selfjwt.ErrTokenInvalid)
		})
	}
}

func TestIssuerAndAudienceRequired(t *testing.T) {
	t.Parallel()
	p := newProvider(t, atlasjwt.AlgEdDSA)
	res, err := selfjwt.NewMinter(p, testIssuer, testAudience, clockOpt(baseTime)).
		Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	tests := []struct {
		name, issuer, audience string
		wantErr                error
	}{
		{name: "empty issuer", audience: testAudience, wantErr: selfjwt.ErrIssuerRequired},
		{name: "empty audience", issuer: testIssuer, wantErr: selfjwt.ErrAudienceRequired},
	}
	for _, tc := range tests {
		// Serial: the subtests share p, whose call counter is not
		// concurrency-safe.
		t.Run(tc.name, func(t *testing.T) {
			_, err := selfjwt.NewMinter(p, tc.issuer, tc.audience, clockOpt(baseTime)).
				Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
			require.ErrorIs(t, err, tc.wantErr)

			_, err = selfjwt.NewVerifier(p, tc.issuer, tc.audience, clockOpt(baseTime)).Verify(t.Context(), res.Token)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}
