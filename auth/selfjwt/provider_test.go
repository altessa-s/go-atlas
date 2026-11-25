// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/jwt"
	"github.com/altessa-s/go-atlas/auth/selfjwt"
)

type publicKey struct{ key jwt.VerificationKey }

func (p publicKey) VerificationKey(context.Context, string, string) (jwt.VerificationKey, error) {
	return p.key, nil
}

func TestSeparateKeyProviders(t *testing.T) {
	t.Parallel()
	provider := newProvider(t, jwt.AlgEdDSA)
	var signing selfjwt.SigningKeyProvider = provider
	m := selfjwt.NewMinter(signing, testIssuer, testAudience)
	v := selfjwt.NewVerifier(publicKey{jwt.VerificationKey{Algorithm: provider.kp.alg, Key: provider.kp.pub}}, testIssuer, testAudience)
	signed, err := m.Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)
	verified, err := v.Verify(t.Context(), signed.Token)
	require.NoError(t, err)
	require.Equal(t, testSubject, verified.Subject)
}
