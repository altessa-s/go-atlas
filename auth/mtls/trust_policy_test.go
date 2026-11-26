// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls_test

import (
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/mtls"
	"github.com/altessa-s/go-atlas/auth/spiffe"
)

// Ancillary validators must not satisfy the trust decision: with any of them
// alone, a certificate from a foreign trust domain under a shared CA would
// become a principal.
func TestAncillaryValidatorsAreNotATrustDecision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts []mtls.Option
	}{
		{name: "empty trust domain validator", opts: []mtls.Option{mtls.WithValidator(mtls.TrustDomainValidator())}},
		{name: "pinned trust domain validator", opts: []mtls.Option{mtls.WithValidator(mtls.TrustDomainValidator("example.org"))}},
		{name: "expiry only", opts: []mtls.Option{mtls.WithValidator(mtls.ExpiryValidator(nil, time.Minute))}},
		{name: "revocation list only", opts: []mtls.Option{mtls.WithValidator(mtls.NewRevocationList().Validator())}},
		{name: "subject pin only", opts: []mtls.Option{mtls.WithValidator(checkedValidator(t)(mtls.SubjectValidator(pkix.Name{CommonName: "billing"})))}},
		{name: "empty trust domains", opts: []mtls.Option{mtls.WithTrustDomains()}},
		{
			name: "empty trust domains with a validator",
			opts: []mtls.Option{mtls.WithTrustDomains(), mtls.WithValidator(mtls.ExpiryValidator(nil, time.Minute))},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, err := mtls.NewAuthenticator(tc.opts...)
			require.ErrorIs(t, err, mtls.ErrNoTrustDomainPolicy)
			require.Nil(t, a)
		})
	}
}

func TestWithTrustDomains(t *testing.T) {
	t.Parallel()
	local := svidCert(t, "spiffe://example.org/sa/billing")
	foreign := svidCert(t, "spiffe://foreign.example/sa/billing")

	a := newAuthenticator(t, mtls.WithTrustDomains("example.org", "other.org"))

	p, err := a.Authenticate(t.Context(), local)
	require.NoError(t, err)
	require.Equal(t, "spiffe://example.org/sa/billing", p.(spiffe.ID).String())

	_, err = a.Authenticate(t.Context(), foreign)
	require.ErrorIs(t, err, mtls.ErrUntrustedDomain)
}

func TestWithTrustDomainsRunsBeforeValidators(t *testing.T) {
	t.Parallel()
	called := false
	a := newAuthenticator(t,
		mtls.WithTrustDomains("example.org"),
		mtls.WithValidator(func(context.Context, *x509.Certificate) error { called = true; return nil }),
	)
	_, err := a.Authenticate(t.Context(), svidCert(t, "spiffe://foreign.example/x"))
	require.ErrorIs(t, err, mtls.ErrUntrustedDomain)
	require.False(t, called)
}

// A pin wins over the opt-in: the insecure option never widens pinned domains.
func TestWithTrustDomainsWinsOverInsecureOptIn(t *testing.T) {
	t.Parallel()
	a := newAuthenticator(t, mtls.WithInsecureAnyTrustDomain(), mtls.WithTrustDomains("example.org"))
	_, err := a.Authenticate(t.Context(), svidCert(t, "spiffe://foreign.example/x"))
	require.ErrorIs(t, err, mtls.ErrUntrustedDomain)
}

// A custom identity does not replace the trust decision: pinned domains still
// require a SPIFFE ID, and a non-SPIFFE certificate needs the explicit opt-in.
func TestCustomIdentityTrustDecision(t *testing.T) {
	t.Parallel()
	dnCert := &x509.Certificate{Subject: pkix.Name{CommonName: "billing"}}
	byCN := mtls.WithIdentity(func(c *x509.Certificate) (any, error) { return c.Subject.CommonName, nil })

	pinned := newAuthenticator(t, byCN, mtls.WithTrustDomains("example.org"))
	_, err := pinned.Authenticate(t.Context(), dnCert)
	require.ErrorIs(t, err, spiffe.ErrNoSVID)

	optIn := newAuthenticator(t, byCN, mtls.WithInsecureAnyTrustDomain(),
		mtls.WithValidator(checkedValidator(t)(mtls.SubjectValidator(pkix.Name{CommonName: "billing"}))))
	p, err := optIn.Authenticate(t.Context(), dnCert)
	require.NoError(t, err)
	require.Equal(t, "billing", p)
}

func TestTrustDomainValidatorWithoutDomainsDeniesAll(t *testing.T) {
	t.Parallel()
	err := mtls.TrustDomainValidator()(t.Context(), svidCert(t, "spiffe://example.org/x"))
	require.ErrorIs(t, err, mtls.ErrNoTrustDomainPolicy)

	a := newAuthenticator(t, mtls.WithInsecureAnyTrustDomain(), mtls.WithValidator(mtls.TrustDomainValidator()))
	_, err = a.Authenticate(t.Context(), svidCert(t, "spiffe://example.org/x"))
	require.ErrorIs(t, err, mtls.ErrNoTrustDomainPolicy)
}
