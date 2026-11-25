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

// TestVerifyNegativeCachesProviderMisses pins that the unverified sub/kid of
// a token cannot drive a key-provider lookup per request: an unknown subject
// or rotated-away kid is remembered for the negative-cache TTL.
func TestVerifyNegativeCachesProviderMisses(t *testing.T) {
	t.Parallel()

	p := newProvider(t, jwt.AlgEdDSA)
	res, err := selfjwt.NewMinter(p, testIssuer, testAudience, selfjwt.WithMinterClock(fixedClock{t: baseTime})).
		Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	tests := []struct {
		name     string
		provider *fakeProvider
		opts     []selfjwt.VerifierOption
		wantErr  error
		calls    int
	}{
		{
			name:     "unknown subject",
			provider: &fakeProvider{subject: "22222222-2222-2222-2222-222222222222", kid: testKeyID, kp: p.kp},
			wantErr:  selfjwt.ErrSubjectUnknown,
			calls:    1,
		},
		{
			name:     "rotated kid",
			provider: &fakeProvider{subject: testSubject, kid: "kid-2", kp: p.kp},
			wantErr:  selfjwt.ErrKeyRotated,
			calls:    1,
		},
		{
			name:     "negative cache disabled",
			provider: &fakeProvider{subject: testSubject, kid: "kid-2", kp: p.kp},
			opts:     []selfjwt.VerifierOption{selfjwt.WithNegativeCacheTTL(0)},
			wantErr:  selfjwt.ErrKeyRotated,
			calls:    2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := append([]selfjwt.VerifierOption{selfjwt.WithVerifierClock(fixedClock{t: baseTime})}, tc.opts...)
			v := selfjwt.NewVerifier(tc.provider, testIssuer, testAudience, opts...)
			for range 2 {
				_, err := v.Verify(t.Context(), res.Token)
				require.ErrorIs(t, err, tc.wantErr)
			}
			require.Equal(t, tc.calls, tc.provider.pubCalls)
		})
	}
}

// TestNegativeCacheExpiresAndInvalidates pins that a negative entry lapses
// after its TTL and is dropped by InvalidateSubject, so a newly provisioned
// subject becomes verifiable.
func TestNegativeCacheExpiresAndInvalidates(t *testing.T) {
	t.Parallel()

	p := newProvider(t, jwt.AlgEdDSA)
	res, err := selfjwt.NewMinter(p, testIssuer, testAudience, selfjwt.WithMinterClock(fixedClock{t: baseTime})).
		Mint(t.Context(), selfjwt.MintRequest{Subject: testSubject, TTL: time.Hour})
	require.NoError(t, err)

	clock := &steppingClock{t: baseTime}
	unknown := &fakeProvider{subject: "22222222-2222-2222-2222-222222222222", kid: testKeyID, kp: p.kp}
	v := selfjwt.NewVerifier(unknown, testIssuer, testAudience, selfjwt.WithVerifierClock(clock))

	_, err = v.Verify(t.Context(), res.Token)
	require.ErrorIs(t, err, selfjwt.ErrSubjectUnknown)

	clock.t = baseTime.Add(selfjwt.DefaultNegativeCacheTTL)
	_, err = v.Verify(t.Context(), res.Token)
	require.ErrorIs(t, err, selfjwt.ErrSubjectUnknown)
	require.Equal(t, 2, unknown.pubCalls, "an expired negative entry must reach the provider")

	unknown.subject = testSubject // the subject is provisioned
	v.InvalidateSubject(testSubject)
	_, err = v.Verify(t.Context(), res.Token)
	require.NoError(t, err)
}

// steppingClock is a selfjwt.Clock a single goroutine moves forward.
type steppingClock struct{ t time.Time }

func (c *steppingClock) Now() time.Time { return c.t }
