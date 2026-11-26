// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/jwt"
	"github.com/altessa-s/go-atlas/auth/selfjwt"
)

// MintResult.Expiry is the instant the signed exp names, not the sub-second
// instant it was computed from.
func TestMintResultExpiryEqualsSignedExp(t *testing.T) {
	t.Parallel()

	p := newProvider(t, jwt.AlgEdDSA)
	for _, now := range []time.Time{
		time.Unix(1_750_000_000, 100_000_000),
		time.Unix(1_750_000_000, 999_999_999),
		time.Unix(1_750_000_000, 0),
	} {
		m := selfjwt.NewMinter(p, testIssuer, testAudience, selfjwt.WithMinterClock(fixedClock{t: now}))
		res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: 1500 * time.Millisecond})
		require.NoError(t, err)

		v := selfjwt.NewVerifier(p, testIssuer, testAudience, selfjwt.WithVerifierClock(fixedClock{t: now}))
		tok, err := v.Verify(t.Context(), res.Token)
		require.NoError(t, err)
		require.True(t, res.Expiry.Equal(tok.Expiry), "minted %s, signed %s", res.Expiry, tok.Expiry)
		require.Zero(t, res.Expiry.Nanosecond())
	}
}
