// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spiffe

import (
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Scheme is the URI scheme of a SPIFFE ID.
const Scheme = "spiffe"

var (
	// ErrInvalidID indicates a string is not a well-formed SPIFFE ID.
	ErrInvalidID = errors.New("spiffe: invalid SPIFFE ID")
	// ErrNoSVID indicates a certificate carries no SPIFFE ID URI SAN.
	ErrNoSVID = errors.New("spiffe: certificate has no SPIFFE ID URI SAN")
	// ErrMultipleURIs indicates a certificate carries more than one URI SAN,
	// which an X.509-SVID must never do.
	ErrMultipleURIs = errors.New("spiffe: certificate has multiple URI SANs")
)

// ID is a parsed SPIFFE ID: a trust domain and a path, as in
// "spiffe://trust-domain/path". The zero value is the empty ID.
type ID struct {
	// TrustDomain is the lowercase authority component, e.g. "example.org".
	TrustDomain string
	// Path is the hierarchical workload path including its leading slash, e.g.
	// "/ns/default/sa/billing". It is empty for a bare trust-domain ID.
	Path string
}

// IsZero reports whether id is the empty ID.
func (id ID) IsZero() bool {
	return id.TrustDomain == "" && id.Path == ""
}

// String renders the canonical "spiffe://trust-domain/path" form, or "" for the
// zero ID.
func (id ID) String() string {
	if id.TrustDomain == "" {
		return ""
	}
	return Scheme + "://" + id.TrustDomain + id.Path
}

// ParseID parses a SPIFFE ID of the form "spiffe://trust-domain/path". It
// enforces the structural rules of the SPIFFE ID spec: the "spiffe" scheme, a
// non-empty trust domain, and no userinfo, port, query, or fragment.
func ParseID(raw string) (ID, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return ID{}, fmt.Errorf("%w: %v", ErrInvalidID, err)
	}
	if !strings.EqualFold(u.Scheme, Scheme) {
		return ID{}, fmt.Errorf("%w: scheme must be %q", ErrInvalidID, Scheme)
	}
	if u.Host == "" {
		return ID{}, fmt.Errorf("%w: missing trust domain", ErrInvalidID)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" {
		return ID{}, fmt.Errorf("%w: must not contain userinfo, port, query, or fragment", ErrInvalidID)
	}
	return ID{TrustDomain: strings.ToLower(u.Host), Path: u.Path}, nil
}

// IDFromCertificate extracts the SPIFFE ID from a leaf certificate. Per the
// X.509-SVID spec a valid SVID carries exactly one URI SAN, and that URI is the
// SPIFFE ID. Zero URIs yields [ErrNoSVID]; more than one yields
// [ErrMultipleURIs].
func IDFromCertificate(cert *x509.Certificate) (ID, error) {
	if cert == nil {
		return ID{}, ErrNoSVID
	}
	switch len(cert.URIs) {
	case 0:
		return ID{}, ErrNoSVID
	case 1:
		return ParseID(cert.URIs[0].String())
	default:
		return ID{}, ErrMultipleURIs
	}
}
