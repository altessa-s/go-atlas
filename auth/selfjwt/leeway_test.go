// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/selfjwt"

	jwt "github.com/altessa-s/go-atlas/auth/jwt"
)

// TestVerifyLeewayCapped pins that WithLeeway is clamped to MaxLeeway, so a
// 24h typo does not extend the lifetime of every token.
func TestVerifyLeewayCapped(t *testing.T) {
	t.Parallel()

	p := newProvider(t, jwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, testIssuer, testAudience, clockOpt(baseTime))
	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Minute})
	require.NoError(t, err)

	tests := []struct {
		name    string
		now     time.Time
		wantErr error
	}{
		{name: "expired beyond MaxLeeway", now: res.Expiry.Add(time.Hour), wantErr: selfjwt.ErrTokenExpired},
		{name: "expired within MaxLeeway", now: res.Expiry.Add(jwt.MaxLeeway - time.Second)},
	}
	for _, tc := range tests {
		// Serial: the subtests share p, whose call counter is not
		// concurrency-safe.
		t.Run(tc.name, func(t *testing.T) {
			v := selfjwt.NewVerifier(p, testIssuer, testAudience, clockOpt(tc.now), selfjwt.WithLeeway(24*time.Hour))
			_, err := v.Verify(t.Context(), res.Token)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
