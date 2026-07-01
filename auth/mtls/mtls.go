// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls

import (
	"context"
	"crypto/x509"
	"errors"
	"time"

	"github.com/altessa-s/go-atlas/auth/audit"
	"github.com/altessa-s/go-atlas/auth/principal"
	"github.com/altessa-s/go-atlas/auth/spiffe"
)

// actionVerifyPeerCert is the audit action recorded for an mTLS authentication.
const actionVerifyPeerCert = "verify_peer_cert"

var (
	// ErrNoCertificate indicates no verified client certificate was supplied to
	// [Authenticator.Authenticate].
	ErrNoCertificate = errors.New("mtls: no client certificate")
)

// IdentityFunc derives a principal from the verified client certificate. The
// returned value becomes the authenticated principal that a downstream
// authorization adapter reads.
type IdentityFunc func(*x509.Certificate) (any, error)

// SPIFFEIdentity is the default [IdentityFunc]: the principal is the
// certificate's SPIFFE ID ([spiffe.ID]). It fails when the certificate carries
// no SPIFFE ID URI SAN.
func SPIFFEIdentity(cert *x509.Certificate) (any, error) {
	return spiffe.IDFromCertificate(cert)
}

// PrincipalIdentity is an opt-in [IdentityFunc] that derives a canonical
// [principal.Principal] (its Subject set to the certificate's SPIFFE ID) for
// callers standardizing on principal.Principal as the authorization subject
// across transports. Wire it with [WithIdentity]; the default stays
// [SPIFFEIdentity]. It fails when the certificate carries no SPIFFE ID URI SAN.
// A certificate carries no scopes or roles, so only Subject is populated.
func PrincipalIdentity(cert *x509.Certificate) (any, error) {
	id, err := spiffe.IDFromCertificate(cert)
	if err != nil {
		return nil, err
	}
	return principal.Principal{Subject: id.String()}, nil
}

type options struct {
	identity   IdentityFunc
	validators []CertValidator
	recorder   *audit.Recorder
	subjectOf  func(any) string
	transport  string
}

// Option configures an [Authenticator].
type Option func(*options)

// WithIdentity overrides how the principal is derived from the verified client
// certificate. The default is [SPIFFEIdentity].
func WithIdentity(fn IdentityFunc) Option {
	return func(o *options) { o.identity = fn }
}

// WithValidator adds one or more [CertValidator] checks run, in order, after the
// principal is derived. A validator that returns an error rejects the caller.
// Use it for checks beyond the TLS handshake — re-validating the certificate
// validity window ([ExpiryValidator]), pinning trust domains
// ([TrustDomainValidator]), or a revocation lookup.
func WithValidator(v ...CertValidator) Option {
	return func(o *options) { o.validators = append(o.validators, v...) }
}

// WithAudit records every authentication decision through rec, with action
// "verify_peer_cert". subjectOf extracts the principal identity for the record;
// pass nil to leave the subject empty.
func WithAudit(rec *audit.Recorder, subjectOf func(any) string) Option {
	return func(o *options) {
		o.recorder = rec
		o.subjectOf = subjectOf
	}
}

// WithTransport labels recorded decisions with the transport that drove the
// authentication ("grpc", "http"). Transport adapters set it; applications
// rarely need to.
func WithTransport(name string) Option {
	return func(o *options) { o.transport = name }
}

// Authenticator turns a verified client certificate into a principal: it derives
// the identity, runs the configured validators, and records an audit decision.
// It is transport-free — a gRPC interceptor or HTTP middleware extracts the
// verified certificate and hands it here. Safe for concurrent use.
type Authenticator struct {
	opts options
}

// NewAuthenticator builds an [Authenticator] from options. The default identity
// is [SPIFFEIdentity] and no extra validators run.
func NewAuthenticator(opts ...Option) *Authenticator {
	o := options{identity: SPIFFEIdentity}
	for _, opt := range opts {
		opt(&o)
	}
	return &Authenticator{opts: o}
}

// Authenticate derives and validates the principal from a verified leaf client
// certificate (the caller extracts it from the transport). A nil certificate is
// rejected. It records an audit decision when a recorder is configured.
//
// On failure it returns the underlying error (the transport adapter maps it to
// its unauthenticated status). Under audit [audit.FailureRequired], a failed
// record on an otherwise-successful authentication is returned wrapping
// [audit.ErrAuditFailed] so the request fails closed.
func (a *Authenticator) Authenticate(ctx context.Context, cert *x509.Certificate) (any, error) {
	if cert == nil {
		return nil, a.finalize(ctx, "", false, "no_certificate", ErrNoCertificate)
	}
	principal, err := a.opts.identity(cert)
	if err != nil {
		return nil, a.finalize(ctx, "", false, reasonOf(err), err)
	}
	for _, validate := range a.opts.validators {
		if verr := validate(cert); verr != nil {
			return nil, a.finalize(ctx, a.subject(principal), false, reasonOf(verr), verr)
		}
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

// finalize records the decision (when a recorder is set) and returns authErr.
// A record failure on an allowed decision under FailureRequired is surfaced so
// the caller fails closed; on a denied decision the record error is ignored
// because the call already fails.
func (a *Authenticator) finalize(ctx context.Context, subject string, allowed bool, reason string, authErr error) error {
	if a.opts.recorder == nil {
		return authErr
	}
	recErr := a.opts.recorder.Record(ctx, audit.Decision{
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

// reasonOf maps an authentication error to a stable, low-cardinality audit token.
func reasonOf(err error) string {
	switch {
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
		return "error"
	}
}
