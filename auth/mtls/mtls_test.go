// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls_test

import (
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/audit"
	"github.com/altessa-s/go-atlas/auth/mtls"
	"github.com/altessa-s/go-atlas/auth/spiffe"
)

func svidCert(t *testing.T, id string) *x509.Certificate {
	t.Helper()
	u, err := url.Parse(id)
	require.NoError(t, err)
	return &x509.Certificate{
		URIs:      []*url.URL{u},
		NotBefore: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:  time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

type captureSink struct {
	got []audit.Decision
	err error
}

func (s *captureSink) Record(_ context.Context, d audit.Decision) error {
	s.got = append(s.got, d)
	return s.err
}

func TestAuthenticateDefaultSPIFFE(t *testing.T) {
	t.Parallel()
	p, err := mtls.NewAuthenticator().Authenticate(t.Context(), svidCert(t, "spiffe://example.org/sa/billing"))
	require.NoError(t, err)
	require.Equal(t, "spiffe://example.org/sa/billing", p.(spiffe.ID).String())
}

func TestAuthenticateNilCert(t *testing.T) {
	t.Parallel()
	_, err := mtls.NewAuthenticator().Authenticate(t.Context(), nil)
	require.ErrorIs(t, err, mtls.ErrNoCertificate)
}

func TestAuthenticateWithIdentity(t *testing.T) {
	t.Parallel()
	a := mtls.NewAuthenticator(mtls.WithIdentity(func(c *x509.Certificate) (any, error) {
		id, err := spiffe.IDFromCertificate(c)
		return id.Path, err
	}))
	p, err := a.Authenticate(t.Context(), svidCert(t, "spiffe://example.org/sa/billing"))
	require.NoError(t, err)
	require.Equal(t, "/sa/billing", p)
}

func TestExpiryValidator(t *testing.T) {
	t.Parallel()
	cert := svidCert(t, "spiffe://example.org/x")
	cert.NotBefore = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	cert.NotAfter = time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

	clock := func(ts string) func() time.Time {
		tt, _ := time.Parse(time.RFC3339, ts)
		return func() time.Time { return tt }
	}

	inWindow := mtls.NewAuthenticator(mtls.WithValidator(mtls.ExpiryValidator(clock("2024-01-15T00:00:00Z"), 0)))
	_, err := inWindow.Authenticate(t.Context(), cert)
	require.NoError(t, err)

	expired := mtls.NewAuthenticator(mtls.WithValidator(mtls.ExpiryValidator(clock("2024-03-01T00:00:00Z"), 0)))
	_, err = expired.Authenticate(t.Context(), cert)
	require.ErrorIs(t, err, mtls.ErrCertExpired)
}

func TestTrustDomainValidator(t *testing.T) {
	t.Parallel()
	cert := svidCert(t, "spiffe://example.org/x")

	ok := mtls.NewAuthenticator(mtls.WithValidator(mtls.TrustDomainValidator("example.org", "other.org")))
	_, err := ok.Authenticate(t.Context(), cert)
	require.NoError(t, err)

	denied := mtls.NewAuthenticator(mtls.WithValidator(mtls.TrustDomainValidator("other.org")))
	_, err = denied.Authenticate(t.Context(), cert)
	require.ErrorIs(t, err, mtls.ErrUntrustedDomain)

	// Empty allowlist is a no-op.
	any := mtls.NewAuthenticator(mtls.WithValidator(mtls.TrustDomainValidator()))
	_, err = any.Authenticate(t.Context(), cert)
	require.NoError(t, err)

	// No SVID surfaces the parse error.
	noSVID := mtls.NewAuthenticator(mtls.WithValidator(mtls.TrustDomainValidator("example.org")))
	_, err = noSVID.Authenticate(t.Context(), &x509.Certificate{})
	require.ErrorIs(t, err, spiffe.ErrNoSVID)
}

func TestRevocationList(t *testing.T) {
	t.Parallel()
	cert := svidCert(t, "spiffe://example.org/x")
	cert.SerialNumber = big.NewInt(42)

	rl := mtls.NewRevocationList()
	a := mtls.NewAuthenticator(mtls.WithValidator(rl.Validator()))

	// Not revoked yet.
	_, err := a.Authenticate(t.Context(), cert)
	require.NoError(t, err)

	// Revoke by serial, then it is rejected.
	rl.Revoke("42")
	require.True(t, rl.IsRevoked("42"))
	_, err = a.Authenticate(t.Context(), cert)
	require.ErrorIs(t, err, mtls.ErrRevoked)

	// Restore lifts the revocation.
	rl.Restore("42")
	_, err = a.Authenticate(t.Context(), cert)
	require.NoError(t, err)

	// Seeded constructor and nil receiver.
	require.True(t, mtls.NewRevocationList("7").IsRevoked("7"))
	var nilRL *mtls.RevocationList
	require.NoError(t, nilRL.Validator()(cert))
}

func TestSubjectValidator(t *testing.T) {
	t.Parallel()
	cert := &x509.Certificate{Subject: pkix.Name{CommonName: "billing", Organization: []string{"Acme"}}}

	require.NoError(t, mtls.SubjectValidator()(cert)) // empty is a no-op
	require.NoError(t, mtls.SubjectValidator(pkix.Name{CommonName: "billing"})(cert))
	// Any-of: matches the second candidate.
	require.NoError(t, mtls.SubjectValidator(pkix.Name{CommonName: "x"}, pkix.Name{CommonName: "billing"})(cert))
	require.ErrorIs(t, mtls.SubjectValidator(pkix.Name{CommonName: "other"})(cert), mtls.ErrSubjectMismatch)
	// Subset: the Organization must also match.
	require.ErrorIs(t,
		mtls.SubjectValidator(pkix.Name{CommonName: "billing", Organization: []string{"Evil"}})(cert),
		mtls.ErrSubjectMismatch)
}

func TestIssuerValidator(t *testing.T) {
	t.Parallel()
	cert := &x509.Certificate{Issuer: pkix.Name{CommonName: "Acme Root CA"}}

	require.NoError(t, mtls.IssuerValidator(pkix.Name{CommonName: "Acme Root CA"})(cert))
	require.ErrorIs(t, mtls.IssuerValidator(pkix.Name{CommonName: "Evil CA"})(cert), mtls.ErrIssuerMismatch)
}

func TestAuthorityKeyIDValidator(t *testing.T) {
	t.Parallel()
	cert := &x509.Certificate{AuthorityKeyId: []byte{1, 2, 3}}

	require.NoError(t, mtls.AuthorityKeyIDValidator()(cert)) // empty is a no-op
	require.NoError(t, mtls.AuthorityKeyIDValidator([]byte{9}, []byte{1, 2, 3})(cert))
	require.ErrorIs(t, mtls.AuthorityKeyIDValidator([]byte{9})(cert), mtls.ErrUntrustedCA)
}

func TestDNSNameValidator(t *testing.T) {
	t.Parallel()
	cert := &x509.Certificate{DNSNames: []string{"api.example.org"}}

	require.NoError(t, mtls.DNSNameValidator()(cert)) // empty is a no-op
	require.NoError(t, mtls.DNSNameValidator("api.example.org")(cert))
	require.NoError(t, mtls.DNSNameValidator("other.org", "api.example.org")(cert)) // any-of
	require.ErrorIs(t, mtls.DNSNameValidator("other.org")(cert), mtls.ErrDNSNameMismatch)

	// Wildcard SANs are honored via VerifyHostname.
	wild := &x509.Certificate{DNSNames: []string{"*.example.org"}}
	require.NoError(t, mtls.DNSNameValidator("api.example.org")(wild))
}

func TestEKUValidator(t *testing.T) {
	t.Parallel()
	cert := &x509.Certificate{ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}

	require.NoError(t, mtls.EKUValidator()(cert)) // empty is a no-op
	require.NoError(t, mtls.EKUValidator(x509.ExtKeyUsageClientAuth)(cert))
	require.ErrorIs(t, mtls.EKUValidator(x509.ExtKeyUsageServerAuth)(cert), mtls.ErrEKUMissing)
	// AND-semantics: missing one required EKU rejects.
	require.ErrorIs(t,
		mtls.EKUValidator(x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth)(cert),
		mtls.ErrEKUMissing)

	// ExtKeyUsageAny satisfies every requirement.
	anyCert := &x509.Certificate{ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}
	require.NoError(t, mtls.EKUValidator(x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth)(anyCert))
}

func subjectOf(p any) string {
	if id, ok := p.(spiffe.ID); ok {
		return id.String()
	}
	return ""
}

func TestAuditRecordsAllowAndDeny(t *testing.T) {
	t.Parallel()
	cert := svidCert(t, "spiffe://example.org/sa/billing")

	t.Run("allow", func(t *testing.T) {
		t.Parallel()
		sink := &captureSink{}
		a := mtls.NewAuthenticator(
			mtls.WithTransport("grpc"),
			mtls.WithAudit(audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll)), subjectOf),
		)
		_, err := a.Authenticate(t.Context(), cert)
		require.NoError(t, err)
		require.Len(t, sink.got, 1)
		require.True(t, sink.got[0].Allowed)
		require.Equal(t, "verify_peer_cert", sink.got[0].Action)
		require.Equal(t, "spiffe://example.org/sa/billing", sink.got[0].Subject)
		require.Equal(t, "grpc", sink.got[0].Attributes["transport"])
	})

	t.Run("deny records reason", func(t *testing.T) {
		t.Parallel()
		sink := &captureSink{}
		a := mtls.NewAuthenticator(
			mtls.WithAudit(audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll)), subjectOf),
			mtls.WithValidator(mtls.TrustDomainValidator("other.org")),
		)
		_, err := a.Authenticate(t.Context(), cert)
		require.ErrorIs(t, err, mtls.ErrUntrustedDomain)
		require.Len(t, sink.got, 1)
		require.False(t, sink.got[0].Allowed)
		require.Equal(t, "untrusted_domain", sink.got[0].Reason)
	})
}

func TestAuditDenyOnlyDropsAllow(t *testing.T) {
	t.Parallel()
	sink := &captureSink{}
	a := mtls.NewAuthenticator(mtls.WithAudit(audit.NewRecorder(sink), subjectOf)) // default deny-only
	_, err := a.Authenticate(t.Context(), svidCert(t, "spiffe://example.org/x"))
	require.NoError(t, err)
	require.Empty(t, sink.got)
}

func TestAuditRequiredFailureFailsClosed(t *testing.T) {
	t.Parallel()
	sink := &captureSink{err: errors.New("sink down")}
	a := mtls.NewAuthenticator(mtls.WithAudit(
		audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll), audit.WithFailureMode(audit.FailureRequired)),
		subjectOf,
	))
	_, err := a.Authenticate(t.Context(), svidCert(t, "spiffe://example.org/x"))
	require.ErrorIs(t, err, audit.ErrAuditFailed)
}
