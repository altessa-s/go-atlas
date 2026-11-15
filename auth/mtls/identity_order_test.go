// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls_test

import (
	"crypto/x509"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit/decision"
	"github.com/altessa-s/go-atlas/auth/mtls"
	"github.com/altessa-s/go-atlas/auth/spiffe"
)

// The identity function may look the peer up or provision it, so it must not
// run for a certificate the trust-domain pin or a validator rejects.
func TestIdentityRunsOnlyForAcceptedCertificate(t *testing.T) {
	t.Parallel()
	past := func() time.Time { return time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC) }

	tests := []struct {
		name    string
		cert    func(t *testing.T) *x509.Certificate
		opts    []mtls.Option
		wantErr error
	}{
		{
			name:    "foreign trust domain",
			cert:    func(t *testing.T) *x509.Certificate { return svidCert(t, "spiffe://foreign.example/x") },
			opts:    []mtls.Option{mtls.WithTrustDomains("example.org")},
			wantErr: mtls.ErrUntrustedDomain,
		},
		{
			name:    "no SPIFFE ID under a pin",
			cert:    func(*testing.T) *x509.Certificate { return &x509.Certificate{} },
			opts:    []mtls.Option{mtls.WithTrustDomains("example.org")},
			wantErr: spiffe.ErrNoSVID,
		},
		{
			name: "outside the validity window",
			cert: func(t *testing.T) *x509.Certificate { return svidCert(t, "spiffe://example.org/x") },
			opts: []mtls.Option{
				mtls.WithTrustDomains("example.org"),
				mtls.WithValidator(mtls.ExpiryValidator(past, 0)),
			},
			wantErr: mtls.ErrCertExpired,
		},
		{
			name: "revoked",
			cert: func(t *testing.T) *x509.Certificate {
				c := svidCert(t, "spiffe://example.org/x")
				c.SerialNumber = big.NewInt(42)
				return c
			},
			opts: []mtls.Option{
				mtls.WithInsecureAnyTrustDomain(),
				mtls.WithValidator(mtls.NewRevocationList("42").Validator()),
			},
			wantErr: mtls.ErrRevoked,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			called := false
			sink := &captureSink{}
			a := newAuthenticator(t, append(tc.opts,
				mtls.WithIdentity(func(*x509.Certificate) (any, error) { called = true; return "caller", nil }),
				mtls.WithAudit(decision.NewRecorder(sink), func(any) string { return "from-identity" }),
			)...)

			_, err := a.Authenticate(t.Context(), tc.cert(t))
			require.ErrorIs(t, err, tc.wantErr)
			require.False(t, called, "the identity function ran for a rejected certificate")
			require.Len(t, sink.got, 1)
			require.NotEqual(t, "from-identity", sink.got[0].Subject)
		})
	}
}

func TestIdentityRunsAfterChecksPass(t *testing.T) {
	t.Parallel()
	var order []string
	a := newAuthenticator(t,
		mtls.WithTrustDomains("example.org"),
		mtls.WithValidator(func(*x509.Certificate) error { order = append(order, "validator"); return nil }),
		mtls.WithIdentity(func(*x509.Certificate) (any, error) { order = append(order, "identity"); return "caller", nil }),
	)
	p, err := a.Authenticate(t.Context(), svidCert(t, "spiffe://example.org/x"))
	require.NoError(t, err)
	require.Equal(t, "caller", p)
	require.Equal(t, []string{"validator", "identity"}, order)
}

// A rejected certificate is audited under its own SPIFFE ID.
func TestRejectedCertificateAuditSubject(t *testing.T) {
	t.Parallel()
	sink := &captureSink{}
	a := newAuthenticator(t, mtls.WithTrustDomains("example.org"), mtls.WithAudit(decision.NewRecorder(sink), subjectOf))
	_, err := a.Authenticate(t.Context(), svidCert(t, "spiffe://foreign.example/sa/x"))
	require.ErrorIs(t, err, mtls.ErrUntrustedDomain)
	require.Len(t, sink.got, 1)
	require.Equal(t, "spiffe://foreign.example/sa/x", sink.got[0].Subject)
}

func TestWithIdentityNilKeepsDefault(t *testing.T) {
	t.Parallel()
	a := newAuthenticator(t, mtls.WithInsecureAnyTrustDomain(), mtls.WithIdentity(nil))
	p, err := a.Authenticate(t.Context(), svidCert(t, "spiffe://example.org/sa/billing"))
	require.NoError(t, err)
	require.Equal(t, "spiffe://example.org/sa/billing", p.(spiffe.ID).String())
}

// Blank entries are not a trust decision, and matching ignores case and
// surrounding space.
func TestWithTrustDomainsNormalizes(t *testing.T) {
	t.Parallel()

	for _, domains := range [][]string{{""}, {"  "}, {"", "\t"}} {
		a, err := mtls.NewAuthenticator(mtls.WithTrustDomains(domains...))
		require.ErrorIs(t, err, mtls.ErrNoTrustDomainPolicy, "%q", domains)
		require.Nil(t, a)
	}

	a := newAuthenticator(t, mtls.WithTrustDomains("", " Example.ORG "))
	_, err := a.Authenticate(t.Context(), svidCert(t, "spiffe://example.org/x"))
	require.NoError(t, err)
	_, err = a.Authenticate(t.Context(), svidCert(t, "spiffe://foreign.example/x"))
	require.ErrorIs(t, err, mtls.ErrUntrustedDomain)
}
