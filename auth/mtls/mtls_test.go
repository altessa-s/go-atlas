// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls_test

import (
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit/decision"
	"github.com/altessa-s/go-atlas/auth/mtls"
	"github.com/altessa-s/go-atlas/auth/spiffe"
)

var _ mtls.DecisionRecorder = (*decision.Recorder)(nil)

type decisionRecorderFunc func(context.Context, decision.Decision) error

func (f decisionRecorderFunc) Record(ctx context.Context, d decision.Decision) error {
	return f(ctx, d)
}

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

// newAuthenticator builds an authenticator from a valid configuration.
func newAuthenticator(tb testing.TB, opts ...mtls.Option) *mtls.Authenticator {
	tb.Helper()
	a, err := mtls.NewAuthenticator(opts...)
	require.NoError(tb, err)
	return a
}

type captureSink struct {
	got []decision.Decision
	err error
}

func (s *captureSink) Record(_ context.Context, d decision.Decision) error {
	s.got = append(s.got, d)
	return s.err
}

func TestAuthenticateDefaultSPIFFE(t *testing.T) {
	t.Parallel()
	a := newAuthenticator(t, mtls.WithInsecureAnyTrustDomain())
	p, err := a.Authenticate(t.Context(), svidCert(t, "spiffe://example.org/sa/billing"))
	require.NoError(t, err)
	require.Equal(t, "spiffe://example.org/sa/billing", p.(spiffe.ID).String())
}

func TestNoTrustDomainPolicyFailsClosed(t *testing.T) {
	t.Parallel()
	cert := svidCert(t, "spiffe://example.org/sa/billing")
	customIdentity := mtls.WithIdentity(func(*x509.Certificate) (any, error) { return "caller", nil })

	tests := []struct {
		name    string
		opts    []mtls.Option
		wantErr error
	}{
		{name: "default options", wantErr: mtls.ErrNoTrustDomainPolicy},
		{name: "custom identity only", opts: []mtls.Option{customIdentity}, wantErr: mtls.ErrNoTrustDomainPolicy},
		{name: "trust domain pinned", opts: []mtls.Option{mtls.WithTrustDomains("example.org")}},
		{name: "explicit opt-in", opts: []mtls.Option{mtls.WithInsecureAnyTrustDomain()}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, err := mtls.NewAuthenticator(tc.opts...)
			require.ErrorIs(t, err, tc.wantErr)
			if tc.wantErr != nil {
				require.Nil(t, a)
				return
			}
			_, err = a.Authenticate(t.Context(), cert)
			require.NoError(t, err)
		})
	}
}

func TestAuthenticateNilCert(t *testing.T) {
	t.Parallel()
	_, err := newAuthenticator(t, mtls.WithInsecureAnyTrustDomain()).Authenticate(t.Context(), nil)
	require.ErrorIs(t, err, mtls.ErrNoCertificate)
}

func TestAuthenticateWithIdentity(t *testing.T) {
	t.Parallel()
	a := newAuthenticator(t, mtls.WithInsecureAnyTrustDomain(), mtls.WithIdentity(func(c *x509.Certificate) (any, error) {
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

	inWindow := newAuthenticator(t, mtls.WithTrustDomains("example.org"),
		mtls.WithValidator(mtls.ExpiryValidator(clock("2024-01-15T00:00:00Z"), 0)))
	_, err := inWindow.Authenticate(t.Context(), cert)
	require.NoError(t, err)

	expired := newAuthenticator(t, mtls.WithTrustDomains("example.org"),
		mtls.WithValidator(mtls.ExpiryValidator(clock("2024-03-01T00:00:00Z"), 0)))
	_, err = expired.Authenticate(t.Context(), cert)
	require.ErrorIs(t, err, mtls.ErrCertExpired)
}

func TestTrustDomainValidator(t *testing.T) {
	t.Parallel()
	cert := svidCert(t, "spiffe://example.org/x")

	require.NoError(t, mtls.TrustDomainValidator("example.org", "other.org")(t.Context(), cert))
	require.ErrorIs(t, mtls.TrustDomainValidator("other.org")(t.Context(), cert), mtls.ErrUntrustedDomain)

	// No SVID surfaces the parse error.
	require.ErrorIs(t, mtls.TrustDomainValidator("example.org")(t.Context(), &x509.Certificate{}), spiffe.ErrNoSVID)
}

func TestRevocationList(t *testing.T) {
	t.Parallel()
	cert := svidCert(t, "spiffe://example.org/x")
	cert.SerialNumber = big.NewInt(42)

	rl := mtls.NewRevocationList()
	a := newAuthenticator(t, mtls.WithTrustDomains("example.org"), mtls.WithValidator(rl.Validator()))

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
	require.NoError(t, nilRL.Validator()(t.Context(), cert))
}

func TestSubjectValidator(t *testing.T) {
	t.Parallel()
	cert := &x509.Certificate{Subject: pkix.Name{CommonName: "billing", Organization: []string{"Acme"}}}

	require.NoError(t, checkedValidator(t)(mtls.SubjectValidator())(t.Context(), cert)) // empty is a no-op
	require.NoError(t, checkedValidator(t)(mtls.SubjectValidator(pkix.Name{CommonName: "billing"}))(t.Context(), cert))
	// Any-of: matches the second candidate.
	require.NoError(t, checkedValidator(t)(mtls.SubjectValidator(pkix.Name{CommonName: "x"}, pkix.Name{CommonName: "billing"}))(t.Context(), cert))
	require.ErrorIs(t, checkedValidator(t)(mtls.SubjectValidator(pkix.Name{CommonName: "other"}))(t.Context(), cert), mtls.ErrSubjectMismatch)
	// Subset: the Organization must also match.
	require.ErrorIs(t,
		checkedValidator(t)(mtls.SubjectValidator(pkix.Name{CommonName: "billing", Organization: []string{"Evil"}}))(t.Context(), cert),
		mtls.ErrSubjectMismatch)
}

func TestIssuerValidator(t *testing.T) {
	t.Parallel()
	cert := &x509.Certificate{Issuer: pkix.Name{CommonName: "Acme Root CA"}}

	require.NoError(t, checkedValidator(t)(mtls.IssuerValidator(pkix.Name{CommonName: "Acme Root CA"}))(t.Context(), cert))
	require.ErrorIs(t, checkedValidator(t)(mtls.IssuerValidator(pkix.Name{CommonName: "Evil CA"}))(t.Context(), cert), mtls.ErrIssuerMismatch)
}

func TestAuthorityKeyIDValidator(t *testing.T) {
	t.Parallel()
	cert := &x509.Certificate{AuthorityKeyId: []byte{1, 2, 3}}

	require.NoError(t, checkedValidator(t)(mtls.AuthorityKeyIDValidator())(t.Context(), cert)) // empty is a no-op
	require.NoError(t, checkedValidator(t)(mtls.AuthorityKeyIDValidator([]byte{9}, []byte{1, 2, 3}))(t.Context(), cert))
	require.ErrorIs(t, checkedValidator(t)(mtls.AuthorityKeyIDValidator([]byte{9}))(t.Context(), cert), mtls.ErrUntrustedCA)
}

func TestDNSNameValidator(t *testing.T) {
	t.Parallel()
	cert := &x509.Certificate{DNSNames: []string{"api.example.org"}}

	require.NoError(t, checkedValidator(t)(mtls.DNSNameValidator())(t.Context(), cert)) // empty is a no-op
	require.NoError(t, checkedValidator(t)(mtls.DNSNameValidator("api.example.org"))(t.Context(), cert))
	require.NoError(t, checkedValidator(t)(mtls.DNSNameValidator("other.org", "api.example.org"))(t.Context(), cert)) // any-of
	require.ErrorIs(t, checkedValidator(t)(mtls.DNSNameValidator("other.org"))(t.Context(), cert), mtls.ErrDNSNameMismatch)

	// Wildcard SANs are honored via VerifyHostname.
	wild := &x509.Certificate{DNSNames: []string{"*.example.org"}}
	require.NoError(t, checkedValidator(t)(mtls.DNSNameValidator("api.example.org"))(t.Context(), wild))
}

func TestEKUValidator(t *testing.T) {
	t.Parallel()
	cert := &x509.Certificate{ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}

	require.NoError(t, mtls.EKUValidator()(t.Context(), cert)) // empty is a no-op
	require.NoError(t, mtls.EKUValidator(x509.ExtKeyUsageClientAuth)(t.Context(), cert))
	require.ErrorIs(t, mtls.EKUValidator(x509.ExtKeyUsageServerAuth)(t.Context(), cert), mtls.ErrEKUMissing)
	// AND-semantics: missing one required EKU rejects.
	require.ErrorIs(t,
		mtls.EKUValidator(x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth)(t.Context(), cert),
		mtls.ErrEKUMissing)

	// ExtKeyUsageAny satisfies every requirement.
	anyCert := &x509.Certificate{ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}
	require.NoError(t, mtls.EKUValidator(x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth)(t.Context(), anyCert))
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
		a := newAuthenticator(t,
			mtls.WithInsecureAnyTrustDomain(),
			mtls.WithTransport("grpc"),
			mtls.WithAudit(decision.NewRecorder(sink, decision.WithPolicyMode(decision.PolicyAll)), subjectOf),
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
		a := newAuthenticator(t,
			mtls.WithAudit(decision.NewRecorder(sink, decision.WithPolicyMode(decision.PolicyAll)), subjectOf),
			mtls.WithTrustDomains("other.org"),
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
	a := newAuthenticator(t, mtls.WithInsecureAnyTrustDomain(),
		mtls.WithAudit(decision.NewRecorder(sink), subjectOf)) // default deny-only
	_, err := a.Authenticate(t.Context(), svidCert(t, "spiffe://example.org/x"))
	require.NoError(t, err)
	require.Empty(t, sink.got)
}

func TestAuditRequiredFailureFailsClosed(t *testing.T) {
	t.Parallel()
	sink := &captureSink{err: errors.New("sink down")}
	a := newAuthenticator(t, mtls.WithInsecureAnyTrustDomain(), mtls.WithAudit(
		decision.NewRecorder(sink, decision.WithPolicyMode(decision.PolicyAll), decision.WithFailureMode(decision.FailureRequired)),
		subjectOf,
	))
	_, err := a.Authenticate(t.Context(), svidCert(t, "spiffe://example.org/x"))
	require.ErrorIs(t, err, decision.ErrAuditFailed)
}

func TestDecisionRecorderPort(t *testing.T) {
	t.Parallel()

	cert := svidCert(t, "spiffe://example.org/sa/billing")
	var got []decision.Decision
	recErr := errors.New("recorder failed")
	recorder := decisionRecorderFunc(func(_ context.Context, d decision.Decision) error {
		got = append(got, d)
		return recErr
	})
	a := newAuthenticator(t, mtls.WithInsecureAnyTrustDomain(), mtls.WithAudit(recorder, subjectOf))
	_, err := a.Authenticate(t.Context(), cert)
	require.ErrorIs(t, err, recErr)
	require.Len(t, got, 1)
	require.True(t, got[0].Allowed)

	// An absent recorder leaves authentication behavior unchanged.
	a = newAuthenticator(t, mtls.WithInsecureAnyTrustDomain(), mtls.WithAudit(nil, subjectOf))
	_, err = a.Authenticate(t.Context(), cert)
	require.NoError(t, err)
}

// TestWithValidatorAppends pins that repeated WithValidator calls accumulate
// validators instead of replacing them.
func TestWithValidatorAppends(t *testing.T) {
	t.Parallel()
	var ran []string
	record := func(name string) mtls.CertValidator {
		return func(context.Context, *x509.Certificate) error {
			ran = append(ran, name)
			return nil
		}
	}
	a := newAuthenticator(t, mtls.WithTrustDomains("example.org"),
		mtls.WithValidator(record("a"), record("b")), mtls.WithValidator(record("c")))
	_, err := a.Authenticate(t.Context(), svidCert(t, "spiffe://example.org/sa/billing"))
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b", "c"}, ran)
}

type policyError struct{ error }

func (policyError) AuditReason() string { return "peer_suspended" }

func TestValidatorContextAndAuditReason(t *testing.T) {
	t.Parallel()
	cause := errors.New("peer lookup denied")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	sink := &captureSink{}
	a := newAuthenticator(t, mtls.WithInsecureAnyTrustDomain(),
		mtls.WithValidator(func(got context.Context, _ *x509.Certificate) error {
			require.Same(t, ctx, got)
			return fmt.Errorf("validator: %w", policyError{cause})
		}),
		mtls.WithAudit(decision.NewRecorder(sink), subjectOf))
	_, err := a.Authenticate(ctx, svidCert(t, "spiffe://example.org/x"))
	require.ErrorAs(t, err, new(policyError))
	require.Len(t, sink.got, 1)
	require.Equal(t, "peer_suspended", sink.got[0].Reason)
}

func checkedValidator(t testing.TB) func(mtls.CertValidator, error) mtls.CertValidator {
	t.Helper()
	return func(validator mtls.CertValidator, err error) mtls.CertValidator {
		t.Helper()
		require.NoError(t, err)
		return validator
	}
}
