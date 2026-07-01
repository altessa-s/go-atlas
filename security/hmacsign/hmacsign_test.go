// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package hmacsign_test

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/hmacsign"
)

func fixedClock(t time.Time) hmacsign.Clock { return func() time.Time { return t } }

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	body := []byte(`{"event":"payment.succeeded","id":"evt_123"}`)
	secret := []byte("whsec_test")

	for _, tc := range []struct {
		name   string
		scheme hmacsign.Scheme
	}{
		{"github", hmacsign.GitHub()},
		{"stripe", hmacsign.Stripe()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			signer := hmacsign.NewSigner(tc.scheme, secret, hmacsign.WithClock(fixedClock(now)))
			verifier := hmacsign.NewVerifier(tc.scheme, secret, hmacsign.WithClock(fixedClock(now)))

			header := signer.Sign(body)
			require.Equal(t, tc.scheme.HeaderName(), signer.HeaderName())
			require.NoError(t, verifier.Verify(header, body))
		})
	}
}

func TestVerifyRejectsTamperedBody(t *testing.T) {
	t.Parallel()

	secret := []byte("whsec_test")
	signer := hmacsign.NewSigner(hmacsign.GitHub(), secret)
	verifier := hmacsign.NewVerifier(hmacsign.GitHub(), secret)

	header := signer.Sign([]byte("original"))
	err := verifier.Verify(header, []byte("tampered"))
	require.ErrorIs(t, err, hmacsign.ErrSignatureMismatch)
}

func TestVerifyRejectsWrongSecret(t *testing.T) {
	t.Parallel()

	body := []byte("payload")
	header := hmacsign.NewSigner(hmacsign.GitHub(), []byte("right")).Sign(body)
	err := hmacsign.NewVerifier(hmacsign.GitHub(), []byte("wrong")).Verify(header, body)
	require.ErrorIs(t, err, hmacsign.ErrSignatureMismatch)
}

func TestVerifyMalformedHeader(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		scheme hmacsign.Scheme
		header string
	}{
		{"github_not_hex", hmacsign.GitHub(), "sha256=zzzz"},
		{"github_empty", hmacsign.GitHub(), ""},
		{"stripe_no_timestamp", hmacsign.Stripe(), "v1=abcd"},
		{"stripe_no_signature", hmacsign.Stripe(), "t=123"},
		{"stripe_bad_timestamp", hmacsign.Stripe(), "t=notanumber,v1=abcd"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := hmacsign.NewVerifier(tc.scheme, []byte("s")).Verify(tc.header, []byte("body"))
			require.ErrorIs(t, err, hmacsign.ErrMalformedSignature)
		})
	}
}

func TestStripeTimestampTolerance(t *testing.T) {
	t.Parallel()

	signedAt := time.Unix(1_700_000_000, 0)
	body := []byte("event")
	secret := []byte("whsec_test")
	header := hmacsign.NewSigner(hmacsign.Stripe(), secret, hmacsign.WithClock(fixedClock(signedAt))).Sign(body)

	// Ten minutes later, outside the default five-minute window.
	late := hmacsign.NewVerifier(hmacsign.Stripe(), secret,
		hmacsign.WithClock(fixedClock(signedAt.Add(10*time.Minute))))
	require.ErrorIs(t, late.Verify(header, body), hmacsign.ErrTimestampOutOfTolerance)

	// A widened tolerance accepts it; the signature itself is still valid.
	wide := hmacsign.NewVerifier(hmacsign.Stripe(), secret,
		hmacsign.WithClock(fixedClock(signedAt.Add(10*time.Minute))),
		hmacsign.WithTolerance(time.Hour))
	require.NoError(t, wide.Verify(header, body))

	// Tolerance 0 disables the timestamp check entirely.
	off := hmacsign.NewVerifier(hmacsign.Stripe(), secret,
		hmacsign.WithClock(fixedClock(signedAt.Add(24*time.Hour))),
		hmacsign.WithTolerance(0))
	require.NoError(t, off.Verify(header, body))
}

func TestSecretRotation(t *testing.T) {
	t.Parallel()

	body := []byte("payload")
	oldSecret := []byte("whsec_old")
	newSecret := []byte("whsec_new")

	// Signed with the old secret; the verifier trusts both during rotation.
	header := hmacsign.NewSigner(hmacsign.GitHub(), oldSecret).Sign(body)
	verifier := hmacsign.NewVerifier(hmacsign.GitHub(), newSecret, hmacsign.WithSecrets(oldSecret))
	require.NoError(t, verifier.Verify(header, body))
}

// plainScheme is a minimal custom [hmacsign.Scheme]: a bare hex digest of the
// body in an "X-Signature" header, no timestamp. It exercises the exported seam.
type plainScheme struct{}

func (plainScheme) HeaderName() string                      { return "X-Signature" }
func (plainScheme) Message(_ time.Time, body []byte) []byte { return body }

func (plainScheme) FormatHeader(_ time.Time, digest []byte) string {
	return hex.EncodeToString(digest)
}

func (plainScheme) ParseHeader(header string) (time.Time, [][]byte, error) {
	raw, err := hex.DecodeString(header)
	if err != nil || len(raw) == 0 {
		return time.Time{}, nil, hmacsign.ErrMalformedSignature
	}
	return time.Time{}, [][]byte{raw}, nil
}

func TestCustomScheme(t *testing.T) {
	t.Parallel()

	var scheme hmacsign.Scheme = plainScheme{}
	body := []byte("payload")
	secret := []byte("s3cr3t")

	header := hmacsign.NewSigner(scheme, secret).Sign(body)
	require.Equal(t, "X-Signature", scheme.HeaderName())
	require.NoError(t, hmacsign.NewVerifier(scheme, secret).Verify(header, body))
	require.ErrorIs(t, hmacsign.NewVerifier(scheme, secret).Verify(header, []byte("tampered")),
		hmacsign.ErrSignatureMismatch)
}

// TestGitHubKnownAnswer pins interop against GitHub's documented example so a
// change to the message construction is caught immediately.
// https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries
func TestGitHubKnownAnswer(t *testing.T) {
	t.Parallel()

	secret := []byte("It's a Secret to Everybody")
	body := []byte("Hello, World!")
	const want = "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17"

	require.Equal(t, want, hmacsign.NewSigner(hmacsign.GitHub(), secret).Sign(body))
	require.NoError(t, hmacsign.NewVerifier(hmacsign.GitHub(), secret).Verify(want, body))
}
