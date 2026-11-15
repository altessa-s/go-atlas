// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas-net/hmacsign"
	"github.com/altessa-s/go-atlas/factory/ext/net/hmacsign"

	webhookconfig "github.com/altessa-s/go-atlas/config/webhook"
)

const (
	testSecret = "whsec_test_secret_0123"
	oldSecret  = "whsec_old_secret_01234"
)

func sign(t *testing.T, secret string, body []byte) string {
	t.Helper()
	s, err := hmacsign.NewSigner(hmacsign.GitHub(), []byte(secret))
	require.NoError(t, err)
	return s.Sign(body)
}

func TestVerifierRoundTrip(t *testing.T) {
	t.Parallel()

	cfg := &webhookconfig.Signature{
		Scheme:    webhookconfig.SchemeGitHub,
		Secret:    testSecret,
		Tolerance: webhookconfig.DefaultTolerance,
	}

	v, err := factory.Verifier(cfg)
	require.NoError(t, err)

	body := []byte("payload")
	require.NoError(t, v.Verify(sign(t, testSecret, body), body))
}

func TestVerifierAcceptsRotationSecret(t *testing.T) {
	t.Parallel()

	cfg := &webhookconfig.Signature{
		Scheme:            webhookconfig.SchemeGitHub,
		Secret:            testSecret,
		AdditionalSecrets: []redacted.RedactedString{oldSecret},
	}

	v, err := factory.Verifier(cfg)
	require.NoError(t, err)

	body := []byte("payload")
	require.NoError(t, v.Verify(sign(t, oldSecret, body), body))
}

func TestSignerRoundTrip(t *testing.T) {
	t.Parallel()

	cfg := &webhookconfig.Signature{Scheme: webhookconfig.SchemeStripe, Secret: testSecret}

	s, err := factory.Signer(cfg)
	require.NoError(t, err)

	v, err := factory.Verifier(cfg)
	require.NoError(t, err)

	body := []byte("event")
	require.NoError(t, v.Verify(s.Sign(body), body))
}

func TestUnknownScheme(t *testing.T) {
	t.Parallel()

	_, err := factory.Verifier(&webhookconfig.Signature{Scheme: "nope", Secret: testSecret})
	require.Error(t, err)
}

func TestNilConfig(t *testing.T) {
	t.Parallel()

	_, err := factory.Verifier(nil)
	require.Error(t, err)
	_, err = factory.Signer(nil)
	require.Error(t, err)
}

// TestInvalidConfigRejected pins that the factory validates the config itself
// instead of trusting the caller to have done so: an unset secret (an env var
// that never resolved) must not produce a verifier keyed by "".
func TestInvalidConfigRejected(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		cfg  webhookconfig.Signature
	}{
		{"empty secret", webhookconfig.Signature{Scheme: webhookconfig.SchemeGitHub}},
		{"negative tolerance", webhookconfig.Signature{
			Scheme: webhookconfig.SchemeStripe, Secret: testSecret, Tolerance: -time.Second,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := factory.Verifier(&tc.cfg)
			require.Error(t, err)
			_, err = factory.Signer(&tc.cfg)
			require.Error(t, err)
		})
	}
}

func TestShortSecretRejected(t *testing.T) {
	t.Parallel()

	cfg := &webhookconfig.Signature{Scheme: webhookconfig.SchemeGitHub, Secret: "short"}
	_, err := factory.Verifier(cfg)
	require.ErrorIs(t, err, hmacsign.ErrSecretTooShort)
	_, err = factory.Signer(cfg)
	require.ErrorIs(t, err, hmacsign.ErrSecretTooShort)

	cfg = &webhookconfig.Signature{
		Scheme: webhookconfig.SchemeGitHub, Secret: testSecret,
		AdditionalSecrets: []redacted.RedactedString{"short"},
	}
	_, err = factory.Verifier(cfg)
	require.ErrorIs(t, err, hmacsign.ErrSecretTooShort)
}

// TestVerifierReplayCache pins the rejectReplays mapping: with it a second
// delivery of an accepted Stripe signature fails with ErrReplayed, without it
// the signature verifies again inside the tolerance window.
func TestVerifierReplayCache(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		rejectReplays bool
		wantErr       error
	}{
		{name: "enabled", rejectReplays: true, wantErr: hmacsign.ErrReplayed},
		{name: "disabled", rejectReplays: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &webhookconfig.Signature{
				Scheme:        webhookconfig.SchemeStripe,
				Secret:        testSecret,
				Tolerance:     webhookconfig.DefaultTolerance,
				RejectReplays: tc.rejectReplays,
			}
			s, err := factory.Signer(cfg)
			require.NoError(t, err)
			v, err := factory.Verifier(cfg)
			require.NoError(t, err)

			body := []byte("event")
			header := s.Sign(body)
			require.NoError(t, v.Verify(header, body))
			err = v.Verify(header, body)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}
