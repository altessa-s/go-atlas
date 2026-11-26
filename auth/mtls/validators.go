// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls

import (
	"bytes"
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/auth/spiffe"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
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
	// ErrUnsupportedDNAttribute indicates a [SubjectValidator] or [IssuerValidator]
	// candidate name carries a non-string attribute value. The constructor
	// rejects such a pattern.
	ErrUnsupportedDNAttribute = errors.New("mtls: unsupported DN attribute in validator configuration")
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
type CertValidator func(context.Context, *x509.Certificate) error

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
	return func(_ context.Context, cert *x509.Certificate) error {
		t := now()
		if t.Add(leeway).Before(cert.NotBefore) || t.Add(-leeway).After(cert.NotAfter) {
			return ErrCertExpired
		}
		return nil
	}
}

// TrustDomainValidator pins the accepted SPIFFE trust domains: it parses the
// certificate's SPIFFE ID and rejects any whose trust domain is not in domains
// with [ErrUntrustedDomain]. A certificate without a SPIFFE ID is rejected with
// the underlying parse error. Domains are trimmed and lower-cased and empty
// entries are dropped, as [WithTrustDomains] does. A validator left without a
// domain pins nothing, so it rejects every certificate with
// [ErrNoTrustDomainPolicy] instead of accepting any trust domain.
//
// It is an ancillary check like every other validator: it does not satisfy the
// trust decision [NewAuthenticator] requires. Use [WithTrustDomains] for that.
func TrustDomainValidator(domains ...string) CertValidator {
	allowed := make([]string, 0, len(domains))
	for _, d := range domains {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			allowed = append(allowed, d)
		}
	}
	if len(allowed) == 0 {
		err := fmt.Errorf("%w: TrustDomainValidator needs at least one trust domain", ErrNoTrustDomainPolicy)
		return func(context.Context, *x509.Certificate) error { return err }
	}
	return func(_ context.Context, cert *x509.Certificate) error {
		return checkTrustDomain(cert, allowed)
	}
}

// checkTrustDomain rejects a certificate whose SPIFFE ID trust domain is not
// in allowed.
func checkTrustDomain(cert *x509.Certificate, allowed []string) error {
	id, err := spiffe.IDFromCertificate(cert)
	if err != nil {
		return err
	}
	if !slices.Contains(allowed, id.TrustDomain) {
		return ErrUntrustedDomain
	}
	return nil
}

// RevocationList is a concurrency-safe, in-memory denylist of revoked certificate
// serial numbers. It is the simple, dependency-free revocation building block:
// a feed (CRL poller, an admin action, an OCSP result you cache) calls [Revoke]
// and [Validator] rejects any certificate whose serial is listed.
//
// The X.509 stapling helpers in [github.com/altessa-s/go-tlsutils/ocsp]
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
	return func(_ context.Context, cert *x509.Certificate) error {
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
// certificate with that CN regardless of other RDNs). Every populated field is
// checked: CommonName, SerialNumber, Organization, OrganizationalUnit, Country,
// Locality, Province, StreetAddress and PostalCode, plus every attribute listed
// in Names and ExtraNames, which must appear in the certificate's parsed
// attributes with the same type and string value (Names and ExtraNames add
// requirements; they never relax a struct field). A pkix.Name copied from a
// parsed certificate therefore matches that certificate. A candidate attribute
// with a non-string value returns [ErrUnsupportedDNAttribute] at construction.
// An empty pattern returns an invalid argument error. An empty names
// list is a no-op. This is the classic-PKI counterpart to [TrustDomainValidator]:
// use it when identity lives in the DN rather than a SPIFFE URI.
func SubjectValidator(names ...pkix.Name) (CertValidator, error) {
	return dnValidator(names, func(c *x509.Certificate) pkix.Name { return c.Subject }, ErrSubjectMismatch)
}

// IssuerValidator pins the accepted issuer (CA) DNs by the same subset rule as
// [SubjectValidator], rejecting a non-matching issuer with [ErrIssuerMismatch]
// and an uncheckable configuration with [ErrUnsupportedDNAttribute]. The TLS
// handshake already verified the certificate chains to a trusted CA; this
// narrows that trust to a specific issuer identity. An empty names list is a
// no-op. An empty pattern returns an invalid argument error.
func IssuerValidator(names ...pkix.Name) (CertValidator, error) {
	return dnValidator(names, func(c *x509.Certificate) pkix.Name { return c.Issuer }, ErrIssuerMismatch)
}

// dnValidator builds the shared Subject/Issuer matcher over a deep copy of
// names, so later mutation of the caller's values cannot change the policy.
func dnValidator(names []pkix.Name, dn func(*x509.Certificate) pkix.Name, mismatch error) (CertValidator, error) {
	allowed := make([]pkix.Name, 0, len(names))
	for _, n := range names {
		if n.CommonName == "" && n.SerialNumber == "" &&
			len(n.Country)+len(n.Organization)+len(n.OrganizationalUnit)+len(n.Locality)+
				len(n.Province)+len(n.StreetAddress)+len(n.PostalCode)+len(n.Names)+len(n.ExtraNames) == 0 {
			return nil, coreerrs.Wrap(coreerrs.ErrInvalidArgument, "empty certificate DN pattern")
		}
		if err := checkDNSupported(n); err != nil {
			return nil, err
		}
		allowed = append(allowed, cloneName(n))
	}
	return func(_ context.Context, cert *x509.Certificate) error {
		if len(allowed) == 0 {
			return nil
		}
		if dnMatchesAny(dn(cert), allowed) {
			return nil
		}
		return mismatch
	}, nil
}

// cloneName deep-copies the parts of n the matcher reads.
func cloneName(n pkix.Name) pkix.Name {
	n.Country = slices.Clone(n.Country)
	n.Organization = slices.Clone(n.Organization)
	n.OrganizationalUnit = slices.Clone(n.OrganizationalUnit)
	n.Locality = slices.Clone(n.Locality)
	n.Province = slices.Clone(n.Province)
	n.StreetAddress = slices.Clone(n.StreetAddress)
	n.PostalCode = slices.Clone(n.PostalCode)
	n.Names = cloneAttributes(n.Names)
	n.ExtraNames = cloneAttributes(n.ExtraNames)
	return n
}

func cloneAttributes(attrs []pkix.AttributeTypeAndValue) []pkix.AttributeTypeAndValue {
	if attrs == nil {
		return nil
	}
	out := make([]pkix.AttributeTypeAndValue, len(attrs))
	for i, a := range attrs {
		out[i] = pkix.AttributeTypeAndValue{Type: slices.Clone(a.Type), Value: a.Value}
	}
	return out
}

// checkDNSupported rejects a candidate attribute the matcher cannot compare:
// only string values are supported.
func checkDNSupported(n pkix.Name) error {
	for _, a := range slices.Concat(n.Names, n.ExtraNames) {
		if _, ok := a.Value.(string); !ok {
			return ErrUnsupportedDNAttribute
		}
	}
	return nil
}

// AuthorityKeyIDValidator pins the accepted issuing-CA Authority Key IDs: it
// accepts a certificate whose AuthorityKeyId equals any of keyIDs and rejects
// the rest with [ErrUntrustedCA]. Combined with the handshake's chain
// verification this is a robust CA pin — the signature is already proven valid,
// and this requires the issuer to be the expected one even when the trust pool
// holds several CAs. An empty key ID is invalid. An empty keyIDs list is a no-op.
func AuthorityKeyIDValidator(keyIDs ...[]byte) (CertValidator, error) {
	allowed := make([][]byte, 0, len(keyIDs))
	for _, k := range keyIDs {
		if len(k) == 0 {
			return nil, coreerrs.Wrap(coreerrs.ErrInvalidArgument, "empty authority key ID")
		}
		allowed = append(allowed, bytes.Clone(k))
	}
	return func(_ context.Context, cert *x509.Certificate) error {
		if len(allowed) == 0 {
			return nil
		}
		for _, k := range allowed {
			if bytes.Equal(cert.AuthorityKeyId, k) {
				return nil
			}
		}
		return ErrUntrustedCA
	}, nil
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
// field in got and every attribute in want.Names / want.ExtraNames is present
// in got. Unset fields in want are ignored, so a caller pins only the RDNs it
// cares about.
func dnSubsetMatch(want, got pkix.Name) bool {
	if want.CommonName != "" && want.CommonName != got.CommonName {
		return false
	}
	if want.SerialNumber != "" && want.SerialNumber != got.SerialNumber {
		return false
	}
	return containsAll(got.Organization, want.Organization) &&
		containsAll(got.OrganizationalUnit, want.OrganizationalUnit) &&
		containsAll(got.Country, want.Country) &&
		containsAll(got.Locality, want.Locality) &&
		containsAll(got.Province, want.Province) &&
		containsAll(got.StreetAddress, want.StreetAddress) &&
		containsAll(got.PostalCode, want.PostalCode) &&
		hasAllAttributes(got, want.Names) &&
		hasAllAttributes(got, want.ExtraNames)
}

// hasAllAttributes reports whether every attribute in want appears in got's
// parsed (Names) or extra (ExtraNames) attributes with the same type and string
// value. Values are known to be strings (see checkDNSupported).
func hasAllAttributes(got pkix.Name, want []pkix.AttributeTypeAndValue) bool {
	for _, w := range want {
		equal := func(g pkix.AttributeTypeAndValue) bool {
			gv, ok := g.Value.(string)
			return ok && g.Type.Equal(w.Type) && gv == w.Value
		}
		if !slices.ContainsFunc(got.Names, equal) && !slices.ContainsFunc(got.ExtraNames, equal) {
			return false
		}
	}
	return true
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
// and ignores the legacy CommonName fallback. An empty name is invalid. An
// empty names list is a no-op.
//
// Because wildcards match, one client certificate with a wildcard SAN
// satisfies every name it covers. Pinning a single client identity this way
// requires a CA that does not issue wildcard client certificates.
func DNSNameValidator(names ...string) (CertValidator, error) {
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return nil, coreerrs.Wrap(coreerrs.ErrInvalidArgument, "empty DNS name pattern")
		}
	}
	allowed := slices.Clone(names)
	return func(_ context.Context, cert *x509.Certificate) error {
		if len(allowed) == 0 {
			return nil
		}
		for _, n := range allowed {
			if cert.VerifyHostname(n) == nil {
				return nil
			}
		}
		return ErrDNSNameMismatch
	}, nil
}

// EKUValidator requires the certificate to assert every extended key usage in
// ekus, rejecting a certificate that is missing any with [ErrEKUMissing]. A
// certificate carrying [x509.ExtKeyUsageAny] satisfies every requirement. Pass
// [x509.ExtKeyUsageClientAuth] on a server verifying its clients, or
// [x509.ExtKeyUsageServerAuth] on a client verifying the server. An empty ekus
// list is a no-op.
func EKUValidator(ekus ...x509.ExtKeyUsage) CertValidator {
	required := slices.Clone(ekus)
	return func(_ context.Context, cert *x509.Certificate) error {
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
