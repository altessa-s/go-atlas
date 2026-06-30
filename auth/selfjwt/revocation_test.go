// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt_test

import (
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
	p := newProvider(t, selfjwt.AlgEdDSA)
	m := selfjwt.NewMinter(p, clockOpt(baseTime))
	dl := denylist.New()
	v := selfjwt.NewVerifier(p, clockOpt(baseTime), selfjwt.WithRevocation(dl))

	res, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	_, err = v.Verify(t.Context(), res.Token)
	require.NoError(t, err) // not revoked yet

	dl.Revoke(res.ID)
	_, err = v.Verify(t.Context(), res.Token)
	require.ErrorIs(t, err, jwt.ErrTokenRevoked)
}
