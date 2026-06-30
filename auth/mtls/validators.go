// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls

import (
	"crypto/x509"
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
