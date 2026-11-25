// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls

import (
	"context"
	"crypto/x509"
	"errors"
	"time"

	"github.com/altessa-s/go-atlas/audit/decision"
	"github.com/altessa-s/go-atlas/auth/principal"
	"github.com/altessa-s/go-atlas/auth/spiffe"
)

// actionVerifyPeerCert is the audit action recorded for an mTLS authentication.
const actionVerifyPeerCert = "verify_peer_cert"

var (
	// ErrNoCertificate indicates no verified client certificate was supplied to
	// [Authenticator.Authenticate].
	ErrNoCertificate = errors.New("mtls: no client certificate")
	// ErrNoTrustDomainPolicy indicates an [Authenticator] was built without a
	// trust decision: no [WithTrustDomains] pin and no
	// [WithInsecureAnyTrustDomain] opt-in. [NewAuthenticator] returns it instead
	// of an authenticator. A [TrustDomainValidator] built without domains
	// rejects every certificate with it.
	ErrNoTrustDomainPolicy = errors.New(
		"mtls: no trust-domain policy: pin the accepted domains with WithTrustDomains or opt in with WithInsecureAnyTrustDomain")
)

// IdentityFunc derives a principal from the verified client certificate. The
// returned value becomes the authenticated principal that a downstream
// authorization adapter reads.
type IdentityFunc func(*x509.Certificate) (any, error)

// defaultSPIFFEIdentity is the default [IdentityFunc]: the principal is the
// certificate's SPIFFE ID ([spiffe.ID]). It fails when the certificate carries
// no SPIFFE ID URI SAN.
func defaultSPIFFEIdentity(cert *x509.Certificate) (any, error) {
	return spiffe.IDFromCertificate(cert)
}

// PrincipalIdentity is an opt-in [IdentityFunc] that derives a canonical
// [principal.Principal] (its Subject set to the certificate's SPIFFE ID) for
// callers standardizing on principal.Principal as the authorization subject
// across transports. Wire it with [WithIdentity]; the default stays
// [defaultSPIFFEIdentity]. It fails when the certificate carries no SPIFFE ID URI SAN.
// A certificate carries no scopes or roles, so only Subject is populated.
func PrincipalIdentity(cert *x509.Certificate) (any, error) {
	id, err := spiffe.IDFromCertificate(cert)
	if err != nil {
		return nil, err
	}
	return principal.Principal{Subject: id.String()}, nil
}

// Authenticator turns a verified client certificate into a principal: it checks
// the trust-domain pin and the configured validators, derives the identity of a
// certificate that passed them, and records an audit decision.
// It is transport-free — a gRPC interceptor or HTTP middleware extracts the
// verified certificate and hands it here. Safe for concurrent use.
type Authenticator struct {
	opts options
}

// NewAuthenticator builds an [Authenticator] from options. The default identity
// is [defaultSPIFFEIdentity].
//
// The caller must make a trust decision: pin the accepted SPIFFE trust domains
// with [WithTrustDomains] or opt in with [WithInsecureAnyTrustDomain].
// Validators added with [WithValidator] do not count, whatever they check.
// Without either the configuration is rejected: NewAuthenticator returns
// [ErrNoTrustDomainPolicy] and no authenticator, so wiring fails at startup.
func NewAuthenticator(opts ...Option) (*Authenticator, error) {
	a := &Authenticator{opts: *newOptions(opts...)}
	if len(a.opts.trustDomains) == 0 && !a.opts.insecureAnyTrustDomain {
		return nil, ErrNoTrustDomainPolicy
	}
	return a, nil
}

// Authenticate validates a verified leaf client certificate (the caller
// extracts it from the transport) and derives its principal. A nil certificate
// is rejected. The trust-domain pin runs first, then the validators in order,
// and the identity function last, only for a certificate that passed them. It
// records an audit decision when a recorder is configured.
//
// On failure it returns the underlying error (the transport adapter maps it to
// its unauthenticated status). Under audit [decision.FailureRequired], a failed
// record on an otherwise-successful authentication is returned wrapping
// [decision.ErrAuditFailed] so the request fails closed.
func (a *Authenticator) Authenticate(ctx context.Context, cert *x509.Certificate) (any, error) {
	if cert == nil {
		return nil, a.finalize(ctx, "", false, "no_certificate", ErrNoCertificate)
	}
	// The certificate checks run before the identity function: a caller-supplied
	// identity may look the peer up or provision it, which must not happen for a
	// certificate that is about to be rejected.
	if len(a.opts.trustDomains) > 0 {
		if terr := checkTrustDomain(cert, a.opts.trustDomains); terr != nil {
			return nil, a.finalize(ctx, certSubject(cert), false, reasonOf(terr), terr)
		}
	}
	for _, validate := range a.opts.validators {
		if verr := validate(ctx, cert); verr != nil {
			return nil, a.finalize(ctx, certSubject(cert), false, reasonOf(verr), verr)
		}
	}
	principal, err := a.opts.identity(cert)
	if err != nil {
		return nil, a.finalize(ctx, certSubject(cert), false, reasonOf(err), err)
	}
	if recErr := a.finalize(ctx, a.subject(principal), true, "", nil); recErr != nil {
		return nil, recErr
	}
	return principal, nil
}

func (a *Authenticator) subject(principal any) string {
	if a.opts.subjectOf == nil {
		return ""
	}
	return a.opts.subjectOf(principal)
}

// certSubject names a rejected certificate in its audit record without the
// identity function: its SPIFFE ID, or its Subject DN when it carries none.
func certSubject(cert *x509.Certificate) string {
	if id, err := spiffe.IDFromCertificate(cert); err == nil {
		return id.String()
	}
	return cert.Subject.String()
}

// finalize records the decision (when a recorder is set) and returns authErr.
// A record failure on an allowed decision under FailureRequired is surfaced so
// the caller fails closed; on a denied decision the record error is ignored
// because the call already fails.
func (a *Authenticator) finalize(ctx context.Context, subject string, allowed bool, reason string, authErr error) error {
	if a.opts.recorder == nil {
		return authErr
	}
	recErr := a.opts.recorder.Record(ctx, decision.Decision{
		Time:       time.Now().UTC(),
		Allowed:    allowed,
		Subject:    subject,
		Action:     actionVerifyPeerCert,
		Reason:     reason,
		Attributes: map[string]string{"transport": a.opts.transport},
	})
	if recErr != nil && authErr == nil {
		return recErr
	}
	return authErr
}

// AuditReasoner can be implemented by validator or identity errors to supply a
// stable, low-cardinality audit token. The token must be safe to log and must
// never contain certificate contents or other request data.
type AuditReasoner interface {
	error
	AuditReason() string
}

// reasonOf maps an authentication error to a stable, low-cardinality audit token.
func reasonOf(err error) string {
	switch {
	case errors.Is(err, ErrNoTrustDomainPolicy):
		return "no_trust_domain_policy"
	case errors.Is(err, ErrNoCertificate):
		return "no_certificate"
	case errors.Is(err, ErrCertExpired):
		return "expired"
	case errors.Is(err, ErrUntrustedDomain):
		return "untrusted_domain"
	case errors.Is(err, ErrRevoked):
		return "revoked"
	case errors.Is(err, ErrSubjectMismatch):
		return "subject_mismatch"
	case errors.Is(err, ErrIssuerMismatch):
		return "issuer_mismatch"
	case errors.Is(err, ErrUntrustedCA):
		return "untrusted_ca"
	case errors.Is(err, ErrDNSNameMismatch):
		return "dns_mismatch"
	case errors.Is(err, ErrEKUMissing):
		return "eku_missing"
	case errors.Is(err, spiffe.ErrNoSVID):
		return "no_svid"
	case errors.Is(err, spiffe.ErrMultipleURIs):
		return "multiple_uris"
	case errors.Is(err, spiffe.ErrInvalidID):
		return "invalid_id"
	default:
		if r, ok := errors.AsType[AuditReasoner](err); ok {
			if reason := r.AuditReason(); reason != "" {
				return reason
			}
		}
		return "error"
	}
}
