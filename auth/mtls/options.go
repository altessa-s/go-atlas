// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls

//go:generate go tool optgen generate

// options configures an [Authenticator].
type options struct {
	// identity derives the principal from the verified client certificate.
	// The default is [defaultSPIFFEIdentity]; a nil function keeps it. It runs
	// last, only for a certificate that passed the trust-domain pin and every
	// validator, so a lookup or provisioning step in it never runs for a
	// certificate that is about to be rejected.
	identity IdentityFunc `optgen:"default=defaultSPIFFEIdentity,notnil"`
	// validators are [CertValidator] checks run, in order, before the principal
	// is derived; a validator that returns an error rejects the caller. Each
	// WithValidator call appends. Use it for checks beyond the TLS handshake —
	// re-validating the certificate validity window ([ExpiryValidator]), DN or
	// CA pins, or a revocation lookup. Validators are ancillary: none of them,
	// [TrustDomainValidator] included, counts as the trust decision
	// [NewAuthenticator] requires.
	validators []CertValidator `opt:"Validator" optgen:"append"`
	// WithTrustDomains pins the accepted SPIFFE trust domains and is the trust
	// decision [NewAuthenticator] requires: a certificate whose SPIFFE ID is in
	// another trust domain is rejected with [ErrUntrustedDomain], and one
	// without a SPIFFE ID with the parse error. The check reads the
	// certificate's URI SAN, independently of [WithIdentity], and runs first.
	// Domains are trimmed and lower-cased and empty entries are dropped; a
	// call that leaves no domain sets nothing, so [NewAuthenticator] fails
	// with [ErrNoTrustDomainPolicy] for an empty or all-blank list alone.
	trustDomains []string `optval:"clone,lower,nonempty"`
	// WithInsecureAnyTrustDomain accepts a verified certificate from any SPIFFE
	// trust domain, and one without a SPIFFE ID when [WithIdentity] derives the
	// principal from something else. [NewAuthenticator] fails with
	// [ErrNoTrustDomainPolicy] without [WithTrustDomains] and without this
	// option, because then any certificate the TLS trust pool
	// accepts — including one issued for a foreign trust domain by a shared or
	// federated CA — would become a principal. Prefer [WithTrustDomains]; use
	// this option only when the trust decision is made elsewhere: the trust
	// pool itself, or DN and CA pins added with [WithValidator]. It has no
	// effect when [WithTrustDomains] pins domains.
	insecureAnyTrustDomain bool
	// Set by WithAudit: recorder and subjectOf are assigned together.
	recorder DecisionRecorder `optgen:"manual"`
	// Set by WithAudit: recorder and subjectOf are assigned together.
	subjectOf func(any) string `optgen:"manual"`
	// transport labels recorded decisions with the transport that drove the
	// authentication ("grpc", "http"). Transport adapters set it;
	// applications rarely need to.
	transport string
}

// WithAudit records every authentication decision through rec, with action
// "verify_peer_cert". subjectOf extracts the principal identity for the record
// of an accepted certificate; pass nil to leave the subject empty. A rejected
// certificate has no principal — the identity function does not run for it —
// so its record carries the certificate's SPIFFE ID, or its Subject DN when it
// has none.
func WithAudit(rec DecisionRecorder, subjectOf func(any) string) Option {
	return func(o *options) {
		o.recorder = rec
		o.subjectOf = subjectOf
	}
}
