// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/jwt"
)

// TestVerifyLeewayCapped pins that WithLeeway cannot extend a token's
// lifetime beyond MaxLeeway: a 24h typo still rejects a token that expired
// an hour ago, while a skew inside MaxLeeway is honored.
func TestVerifyLeewayCapped(t *testing.T) {
	t.Parallel()

	ks := newKeysets(t)[0]
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		exp     time.Time
		wantErr error
	}{
		{name: "expired beyond MaxLeeway", exp: now.Add(-time.Hour), wantErr: jwt.ErrTokenExpired},
		{name: "expired within MaxLeeway", exp: now.Add(-jwt.MaxLeeway + time.Second)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw, err := jwt.NewSigner().Sign(ks.signing, jwt.Claims{"sub": "s", "exp": tc.exp.Unix()})
			require.NoError(t, err)

			verifier := jwt.NewVerifier(jwt.StaticKey(ks.verification),
				jwt.WithClock(fakeClock{now}), jwt.WithLeeway(24*time.Hour))
			_, err = verifier.Verify(t.Context(), raw)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
