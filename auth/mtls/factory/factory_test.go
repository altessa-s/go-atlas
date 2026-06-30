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

	"github.com/altessa-s/go-atlas/auth/mtls/factory"
	"github.com/altessa-s/go-atlas/auth/spiffe"
	"github.com/altessa-s/go-atlas/config"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
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
	opts, err := factory.New(&config.MTLS{
		TrustDomains: []string{"example.org"},
		CheckExpiry:  true,
		ExpiryLeeway: 0,
	}).Options()
	require.NoError(t, err)

	a := coremtls.NewAuthenticator(opts...)
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
	opts, err := factory.New(&config.MTLS{
		AllowedSubjectCNs: []string{"billing"},
		IssuerKeyIDs:      []string{"0a0b"},
	}).Options()
	require.NoError(t, err)
	a := coremtls.NewAuthenticator(opts...)

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
	_, err := factory.New(&config.MTLS{IssuerKeyIDs: []string{"zz"}}).Options()
	require.Error(t, err)
}

func TestOptionsEnforceDNSAndEKU(t *testing.T) {
	t.Parallel()
	opts, err := factory.New(&config.MTLS{
		AllowedDNSNames: []string{"api.example.org"},
		RequiredEKUs:    []string{"clientAuth"},
	}).Options()
	require.NoError(t, err)
	a := coremtls.NewAuthenticator(opts...)

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
	_, err := factory.New(&config.MTLS{RequiredEKUs: []string{"bogus"}}).Options()
	require.Error(t, err)
}

func TestOptionsNoValidatorsWhenDisabled(t *testing.T) {
	t.Parallel()
	opts, err := factory.New(&config.MTLS{CheckExpiry: false}).Options()
	require.NoError(t, err)
	require.Empty(t, opts)
}

func TestAuthenticatorAddsExtraOptions(t *testing.T) {
	t.Parallel()
	a, err := factory.New(&config.MTLS{CheckExpiry: false}).Authenticator(
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
