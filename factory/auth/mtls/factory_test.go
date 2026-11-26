// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/spiffe"
	"github.com/altessa-s/go-atlas/factory/auth/mtls"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
	authconfig "github.com/altessa-s/go-atlas/config/auth"
)

func cert(t *testing.T, id string, notAfter time.Time) *x509.Certificate {
	t.Helper()
	u, err := url.Parse(id)
	require.NoError(t, err)
	return &x509.Certificate{
		URIs:      []*url.URL{u},
		NotBefore: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:  notAfter,
	}
}

func TestOptionsNilConfig(t *testing.T) {
	t.Parallel()
	_, err := factory.New(nil).Options()
	require.Error(t, err)
}

func TestOptionsEnforceTrustDomainAndExpiry(t *testing.T) {
	t.Parallel()
	opts, err := factory.New(&authconfig.MTLS{
		TrustDomains: []string{"example.org"},
		CheckExpiry:  true,
		ExpiryLeeway: 0,
	}).Options()
	require.NoError(t, err)

	a, err := coremtls.NewAuthenticator(opts...)
	require.NoError(t, err)
	farFuture := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	farPast := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

	// valid trust domain, not expired
	p, err := a.Authenticate(t.Context(), cert(t, "spiffe://example.org/x", farFuture))
	require.NoError(t, err)
	require.Equal(t, "spiffe://example.org/x", p.(spiffe.ID).String())

	// wrong trust domain
	_, err = a.Authenticate(t.Context(), cert(t, "spiffe://evil.example/x", farFuture))
	require.ErrorIs(t, err, coremtls.ErrUntrustedDomain)

	// expired
	_, err = a.Authenticate(t.Context(), cert(t, "spiffe://example.org/x", farPast))
	require.ErrorIs(t, err, coremtls.ErrCertExpired)
}

func TestOptionsEnforceSubjectAndCA(t *testing.T) {
	t.Parallel()
	opts, err := factory.New(&authconfig.MTLS{
		AllowAnyTrustDomain: true,
		AllowedSubjectCNs:   []string{"billing"},
		IssuerKeyIDs:        []string{"0a0b"},
	}).Options()
	require.NoError(t, err)
	a, err := coremtls.NewAuthenticator(opts...)
	require.NoError(t, err)

	withDN := func(cn string, aki []byte) *x509.Certificate {
		c := cert(t, "spiffe://example.org/x", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
		c.Subject = pkix.Name{CommonName: cn}
		c.AuthorityKeyId = aki
		return c
	}

	_, err = a.Authenticate(t.Context(), withDN("billing", []byte{0x0a, 0x0b}))
	require.NoError(t, err)

	_, err = a.Authenticate(t.Context(), withDN("other", []byte{0x0a, 0x0b}))
	require.ErrorIs(t, err, coremtls.ErrSubjectMismatch)

	_, err = a.Authenticate(t.Context(), withDN("billing", []byte{0xff}))
	require.ErrorIs(t, err, coremtls.ErrUntrustedCA)
}

func TestOptionsInvalidIssuerKeyID(t *testing.T) {
	t.Parallel()
	_, err := factory.New(&authconfig.MTLS{AllowAnyTrustDomain: true, IssuerKeyIDs: []string{"zz"}}).Options()
	require.Error(t, err)
}

func TestOptionsEnforceDNSAndEKU(t *testing.T) {
	t.Parallel()
	opts, err := factory.New(&authconfig.MTLS{
		AllowAnyTrustDomain: true,
		AllowedDNSNames:     []string{"api.example.org"},
		RequiredEKUs:        []string{"clientAuth"},
	}).Options()
	require.NoError(t, err)
	a, err := coremtls.NewAuthenticator(opts...)
	require.NoError(t, err)

	withSAN := func(dns string, eku x509.ExtKeyUsage) *x509.Certificate {
		c := cert(t, "spiffe://example.org/x", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC))
		c.DNSNames = []string{dns}
		c.ExtKeyUsage = []x509.ExtKeyUsage{eku}
		return c
	}

	_, err = a.Authenticate(t.Context(), withSAN("api.example.org", x509.ExtKeyUsageClientAuth))
	require.NoError(t, err)

	_, err = a.Authenticate(t.Context(), withSAN("evil.org", x509.ExtKeyUsageClientAuth))
	require.ErrorIs(t, err, coremtls.ErrDNSNameMismatch)

	_, err = a.Authenticate(t.Context(), withSAN("api.example.org", x509.ExtKeyUsageServerAuth))
	require.ErrorIs(t, err, coremtls.ErrEKUMissing)
}

func TestOptionsUnknownEKU(t *testing.T) {
	t.Parallel()
	_, err := factory.New(&authconfig.MTLS{AllowAnyTrustDomain: true, RequiredEKUs: []string{"bogus"}}).Options()
	require.Error(t, err)
}

func TestOptionsRequireTrustDomainDecision(t *testing.T) {
	t.Parallel()
	farFuture := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		cfg  authconfig.MTLS
	}{
		{name: "empty config", cfg: authconfig.MTLS{}},
		{name: "default config", cfg: authconfig.DefaultMTLS()},
		{name: "other pins only", cfg: authconfig.MTLS{AllowedSubjectCNs: []string{"billing"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := factory.New(&tc.cfg).Options()
			require.ErrorIs(t, err, coremtls.ErrNoTrustDomainPolicy)
			_, err = factory.New(&tc.cfg).Authenticator()
			require.ErrorIs(t, err, coremtls.ErrNoTrustDomainPolicy)
		})
	}

	t.Run("explicit opt-in accepts any trust domain", func(t *testing.T) {
		t.Parallel()
		a, err := factory.New(&authconfig.MTLS{AllowAnyTrustDomain: true}).Authenticator()
		require.NoError(t, err)
		_, err = a.Authenticate(t.Context(), cert(t, "spiffe://anything.example/x", farFuture))
		require.NoError(t, err)
	})

	t.Run("trust domains win over the opt-in", func(t *testing.T) {
		t.Parallel()
		a, err := factory.New(&authconfig.MTLS{
			TrustDomains:        []string{"example.org"},
			AllowAnyTrustDomain: true,
		}).Authenticator()
		require.NoError(t, err)
		_, err = a.Authenticate(t.Context(), cert(t, "spiffe://evil.example/x", farFuture))
		require.ErrorIs(t, err, coremtls.ErrUntrustedDomain)
	})
}

func TestAuthenticatorAddsExtraOptions(t *testing.T) {
	t.Parallel()
	a, err := factory.New(&authconfig.MTLS{AllowAnyTrustDomain: true, CheckExpiry: false}).Authenticator(
		coremtls.WithIdentity(func(c *x509.Certificate) (any, error) {
			id, err := spiffe.IDFromCertificate(c)
			return id.TrustDomain, err
		}),
	)
	require.NoError(t, err)
	p, err := a.Authenticate(t.Context(), cert(t, "spiffe://example.org/x", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)))
	require.NoError(t, err)
	require.Equal(t, "example.org", p)
}

func TestBlankPinsRejected(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		set  func(*authconfig.MTLS)
	}{
		{"subject CN", func(c *authconfig.MTLS) { c.AllowedSubjectCNs = []string{""} }},
		{"issuer CN", func(c *authconfig.MTLS) { c.AllowedIssuerCNs = []string{""} }},
		{"issuer key ID", func(c *authconfig.MTLS) { c.IssuerKeyIDs = []string{""} }},
		{"DNS name", func(c *authconfig.MTLS) { c.AllowedDNSNames = []string{""} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &authconfig.MTLS{AllowAnyTrustDomain: true}
			tc.set(cfg)
			_, err := factory.New(cfg).Options()
			require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
			require.Error(t, cfg.Validate())
		})
	}
}
