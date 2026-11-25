// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mtls turns a verified mutual-TLS client certificate into an
// authenticated principal. It is the transport-free policy core behind the gRPC
// and HTTP mTLS adapters: an [Authenticator] derives the identity from the
// certificate, runs extra validators, and records an audit decision. The
// transport adapters only extract the verified certificate and map a failure to
// their status code.
//
// The default identity is the certificate's SPIFFE ID (see
// [github.com/altessa-s/go-atlas/auth/spiffe]); override it with [WithIdentity].
//
// # Trust decision
//
// The TLS trust pool alone does not say which identities a service accepts: a
// shared or federated CA can issue a certificate for a foreign SPIFFE trust
// domain. [NewAuthenticator] therefore requires an explicit decision — the
// accepted trust domains pinned with [WithTrustDomains], or the
// [WithInsecureAnyTrustDomain] opt-in. Without either, [NewAuthenticator]
// returns [ErrNoTrustDomainPolicy] and no authenticator, so a misconfigured
// service fails at startup. Validators added with
// [WithValidator] are ancillary and never count as that decision. When identity
// is not a SPIFFE ID ([WithIdentity] reading the Subject DN), make the decision
// with [WithInsecureAnyTrustDomain] and pin the identity with [SubjectValidator],
// [IssuerValidator] or [AuthorityKeyIDValidator].
//
// # Order of checks
//
// [Authenticator.Authenticate] checks the trust-domain pin first, then the
// validators in order, and calls the identity function last, only for a
// certificate that passed them. An identity function with side effects — a
// lookup, just-in-time provisioning — never runs for a certificate that is
// rejected.
//
// # Validation beyond TLS
//
// The TLS handshake already verifies the certificate chain and validity window
// at connection time. Validators added with [WithValidator] run on every call,
// which matters for long-lived connections: [ExpiryValidator] re-checks the
// validity window so a connection that outlives its certificate is rejected. A
// revocation lookup (CRL/OCSP/denylist) is a caller-supplied [CertValidator].
// Validators receive the authentication context. Errors implementing [AuditReasoner]
// can supply a stable audit reason for custom failures.
// [SubjectValidator] and [IssuerValidator] pin every populated DN field and
// Names/ExtraNames string attribute. Unsupported attribute values fail closed
// with [ErrUnsupportedDNAttribute].
//
// # Usage
//
//	auth, err := mtls.NewAuthenticator(
//	    mtls.WithTrustDomains("example.org"),
//	    mtls.WithValidator(mtls.ExpiryValidator(nil, 30*time.Second)),
//	    mtls.WithAudit(rec, func(p any) string { id, _ := p.(spiffe.ID); return id.String() }),
//	)
//	if err != nil {
//	    return err
//	}
//	principal, err := auth.Authenticate(ctx, verifiedCert)
package mtls
