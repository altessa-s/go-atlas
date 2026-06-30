// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/auth/spiffe"
)

var (
	// ErrCertExpired indicates the certificate's validity window does not include
	// the current time. Reported by [ExpiryValidator].
	ErrCertExpired = errors.New("mtls: client certificate expired or not yet valid")
	// ErrUntrustedDomain indicates the certificate's SPIFFE ID trust domain is not
	// in the allowed set. Reported by [TrustDomainValidator].
	ErrUntrustedDomain = errors.New("mtls: untrusted SPIFFE trust domain")
	// ErrRevoked indicates the certificate's serial number is on a [RevocationList].
	ErrRevoked = errors.New("mtls: client certificate revoked")
	// ErrSubjectMismatch indicates the certificate's Subject DN matches none of the
	// allowed names. Reported by [SubjectValidator].
	ErrSubjectMismatch = errors.New("mtls: client certificate subject not allowed")
	// ErrIssuerMismatch indicates the certificate's Issuer DN matches none of the
	// allowed names. Reported by [IssuerValidator].
	ErrIssuerMismatch = errors.New("mtls: client certificate issuer not allowed")
	// ErrUntrustedCA indicates the certificate's issuing-CA Authority Key ID is not
	// in the allowed set. Reported by [AuthorityKeyIDValidator].
	ErrUntrustedCA = errors.New("mtls: client certificate issuing CA not allowed")
	// ErrDNSNameMismatch indicates the certificate is valid for none of the allowed
	// DNS names. Reported by [DNSNameValidator].
	ErrDNSNameMismatch = errors.New("mtls: client certificate DNS name not allowed")
	// ErrEKUMissing indicates the certificate does not assert a required extended
	// key usage. Reported by [EKUValidator].
	ErrEKUMissing = errors.New("mtls: client certificate missing required extended key usage")
)

// CertValidator runs an extra check on the verified certificate, beyond what the
// TLS handshake already performed. Returning an error rejects the caller.
type CertValidator func(*x509.Certificate) error

// ExpiryValidator re-checks the certificate validity window at authentication
// time. The TLS handshake already rejects an expired certificate when the
// connection is established, but a long-lived connection (a stream, a pooled
// HTTP/2 connection) can outlive its client certificate; this validator rejects
// such a caller on the next call. now defaults to [time.Now]; leeway tolerates
// small clock skew on both bounds.
func ExpiryValidator(now func() time.Time, leeway time.Duration) CertValidator {
	if now == nil {
		now = time.Now
	}
	return func(cert *x509.Certificate) error {
		t := now()
		if t.Add(leeway).Before(cert.NotBefore) || t.Add(-leeway).After(cert.NotAfter) {
			return ErrCertExpired
		}
		return nil
	}
}

// TrustDomainValidator pins the accepted SPIFFE trust domains: it parses the
// certificate's SPIFFE ID and rejects any whose trust domain is not in domains.
// An empty domains list accepts any trust domain (the validator is a no-op). A
// certificate without a SPIFFE ID is rejected with the underlying parse error.
func TrustDomainValidator(domains ...string) CertValidator {
	allowed := slices.Clone(domains)
	return func(cert *x509.Certificate) error {
		if len(allowed) == 0 {
			return nil
		}
		id, err := spiffe.IDFromCertificate(cert)
		if err != nil {
			return err
		}
		if !slices.Contains(allowed, id.TrustDomain) {
			return ErrUntrustedDomain
		}
		return nil
	}
}

// RevocationList is a concurrency-safe, in-memory denylist of revoked certificate
// serial numbers. It is the simple, dependency-free revocation building block:
// a feed (CRL poller, an admin action, an OCSP result you cache) calls [Revoke]
// and [Validator] rejects any certificate whose serial is listed.
//
// The X.509 stapling helpers in [github.com/altessa-s/go-atlas/security/tlsutils/ocsp]
// staple the server's own certificate status during the handshake; they do not
// answer "is this peer certificate revoked". For richer client-revocation
// (live OCSP, full CRL semantics) write your own [CertValidator].
type RevocationList struct {
	mu      sync.RWMutex
	revoked map[string]struct{}
}

// NewRevocationList builds a list seeded with the given serial numbers (decimal,
// as from cert.SerialNumber.String()).
func NewRevocationList(serials ...string) *RevocationList {
	rl := &RevocationList{revoked: make(map[string]struct{}, len(serials))}
	for _, s := range serials {
		rl.revoked[s] = struct{}{}
	}
	return rl
}

// Revoke adds a serial number to the list.
func (rl *RevocationList) Revoke(serial string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.revoked[serial] = struct{}{}
}

// Restore removes a serial number from the list.
func (rl *RevocationList) Restore(serial string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.revoked, serial)
}

// IsRevoked reports whether the serial number is on the list.
func (rl *RevocationList) IsRevoked(serial string) bool {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	_, ok := rl.revoked[serial]
	return ok
}

// Validator returns a [CertValidator] that rejects a certificate whose serial
// number is on the list with [ErrRevoked]. A nil receiver is a no-op validator.
func (rl *RevocationList) Validator() CertValidator {
	return func(cert *x509.Certificate) error {
		if rl == nil || cert.SerialNumber == nil {
			return nil
		}
		if rl.IsRevoked(cert.SerialNumber.String()) {
			return ErrRevoked
		}
		return nil
	}
}

// SubjectValidator pins the accepted certificate Subject DNs: it accepts a
// certificate whose Subject matches any of names and rejects the rest with
// [ErrSubjectMismatch]. Matching is subset-based — only the fields set in a
// candidate name must match (so pkix.Name{CommonName: "billing"} matches any
// certificate with that CN regardless of other RDNs). An empty names list is a
// no-op. This is the classic-PKI counterpart to [TrustDomainValidator]: use it
// when identity lives in the DN rather than a SPIFFE URI.
func SubjectValidator(names ...pkix.Name) CertValidator {
	allowed := slices.Clone(names)
	return func(cert *x509.Certificate) error {
		if len(allowed) == 0 {
			return nil
		}
		if dnMatchesAny(cert.Subject, allowed) {
			return nil
		}
		return ErrSubjectMismatch
	}
}

// IssuerValidator pins the accepted issuer (CA) DNs by the same subset rule as
// [SubjectValidator], rejecting a non-matching issuer with [ErrIssuerMismatch].
// The TLS handshake already verified the certificate chains to a trusted CA;
// this narrows that trust to a specific issuer identity. An empty names list is
// a no-op.
func IssuerValidator(names ...pkix.Name) CertValidator {
	allowed := slices.Clone(names)
	return func(cert *x509.Certificate) error {
		if len(allowed) == 0 {
			return nil
		}
		if dnMatchesAny(cert.Issuer, allowed) {
			return nil
		}
		return ErrIssuerMismatch
	}
}

// AuthorityKeyIDValidator pins the accepted issuing-CA Authority Key IDs: it
// accepts a certificate whose AuthorityKeyId equals any of keyIDs and rejects
// the rest with [ErrUntrustedCA]. Combined with the handshake's chain
// verification this is a robust CA pin — the signature is already proven valid,
// and this requires the issuer to be the expected one even when the trust pool
// holds several CAs. An empty keyIDs list is a no-op.
func AuthorityKeyIDValidator(keyIDs ...[]byte) CertValidator {
	allowed := make([][]byte, 0, len(keyIDs))
	for _, k := range keyIDs {
		allowed = append(allowed, bytes.Clone(k))
	}
	return func(cert *x509.Certificate) error {
		if len(allowed) == 0 {
			return nil
		}
		for _, k := range allowed {
			if bytes.Equal(cert.AuthorityKeyId, k) {
				return nil
			}
		}
		return ErrUntrustedCA
	}
}

// dnMatchesAny reports whether got matches any of the candidate names.
func dnMatchesAny(got pkix.Name, want []pkix.Name) bool {
	for _, w := range want {
		if dnSubsetMatch(w, got) {
			return true
		}
	}
	return false
}

// dnSubsetMatch reports whether every field set in want equals the matching
// field in got. Unset fields in want are ignored, so a caller pins only the RDNs
// it cares about.
func dnSubsetMatch(want, got pkix.Name) bool {
	if want.CommonName != "" && want.CommonName != got.CommonName {
		return false
	}
	if want.SerialNumber != "" && want.SerialNumber != got.SerialNumber {
		return false
	}
	return containsAll(got.Organization, want.Organization) &&
		containsAll(got.OrganizationalUnit, want.OrganizationalUnit) &&
		containsAll(got.Country, want.Country)
}

// containsAll reports whether every needle is present in haystack.
func containsAll(haystack, needles []string) bool {
	for _, n := range needles {
		if !slices.Contains(haystack, n) {
			return false
		}
	}
	return true
}

// DNSNameValidator pins the accepted DNS Subject Alternative Names: it accepts a
// certificate that is valid for any of names and rejects the rest with
// [ErrDNSNameMismatch]. Matching uses [x509.Certificate.VerifyHostname], so it
// honors wildcard SANs (a cert with "*.example.org" satisfies "api.example.org")
// and ignores the legacy CommonName fallback. An empty names list is a no-op.
func DNSNameValidator(names ...string) CertValidator {
	allowed := slices.Clone(names)
	return func(cert *x509.Certificate) error {
		if len(allowed) == 0 {
			return nil
		}
		for _, n := range allowed {
			if cert.VerifyHostname(n) == nil {
				return nil
			}
		}
		return ErrDNSNameMismatch
	}
}

// EKUValidator requires the certificate to assert every extended key usage in
// ekus, rejecting a certificate that is missing any with [ErrEKUMissing]. A
// certificate carrying [x509.ExtKeyUsageAny] satisfies every requirement. Pass
// [x509.ExtKeyUsageClientAuth] on a server verifying its clients, or
// [x509.ExtKeyUsageServerAuth] on a client verifying the server. An empty ekus
// list is a no-op.
func EKUValidator(ekus ...x509.ExtKeyUsage) CertValidator {
	required := slices.Clone(ekus)
	return func(cert *x509.Certificate) error {
		for _, want := range required {
			if !hasEKU(cert, want) {
				return ErrEKUMissing
			}
		}
		return nil
	}
}

// hasEKU reports whether cert asserts want (or the catch-all ExtKeyUsageAny).
func hasEKU(cert *x509.Certificate, want x509.ExtKeyUsage) bool {
	for _, e := range cert.ExtKeyUsage {
		if e == want || e == x509.ExtKeyUsageAny {
			return true
		}
	}
	return false
}
