// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/security/hmacsign"
	"github.com/altessa-s/go-atlas/security/hmacsign/factory"
)

func TestVerifierRoundTrip(t *testing.T) {
	t.Parallel()

	cfg := &config.WebhookSignature{
		Scheme:    config.WebhookSchemeGitHub,
		Secret:    "whsec_test",
		Tolerance: config.DefaultWebhookTolerance,
	}

	v, err := factory.Verifier(cfg)
	require.NoError(t, err)

	body := []byte("payload")
	header := hmacsign.NewSigner(hmacsign.GitHub(), []byte("whsec_test")).Sign(body)
	require.NoError(t, v.Verify(header, body))
}

func TestVerifierAcceptsRotationSecret(t *testing.T) {
	t.Parallel()

	cfg := &config.WebhookSignature{
		Scheme:            config.WebhookSchemeGitHub,
		Secret:            "whsec_new",
		AdditionalSecrets: []config.Secret{"whsec_old"},
	}

	v, err := factory.Verifier(cfg)
	require.NoError(t, err)

	body := []byte("payload")
	header := hmacsign.NewSigner(hmacsign.GitHub(), []byte("whsec_old")).Sign(body)
	require.NoError(t, v.Verify(header, body))
}

func TestSignerRoundTrip(t *testing.T) {
	t.Parallel()

	cfg := &config.WebhookSignature{Scheme: config.WebhookSchemeStripe, Secret: "whsec_test"}

	s, err := factory.Signer(cfg)
	require.NoError(t, err)

	v, err := factory.Verifier(cfg)
	require.NoError(t, err)

	body := []byte("event")
	require.NoError(t, v.Verify(s.Sign(body), body))
}

func TestUnknownScheme(t *testing.T) {
	t.Parallel()

	_, err := factory.Verifier(&config.WebhookSignature{Scheme: "nope", Secret: "s"})
	require.Error(t, err)
}

func TestNilConfig(t *testing.T) {
	t.Parallel()

	_, err := factory.Verifier(nil)
	require.Error(t, err)
}
