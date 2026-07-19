// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtlstest

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/url"
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// CA is a self-signed certificate authority for mutual-TLS integration tests.
// It is a thin wrapper over [testhelpers.CA] adding server- and client-leaf
// conveniences; the embedded Cert, Key, and Pool fields are promoted — enough
// to stand up a real handshake on both ends (server ClientCAs, client
// RootCAs).
type CA struct {
	*testhelpers.CA
}

// NewCA generates a fresh in-memory CA. The certificate is valid for one hour,
// pre-dated to tolerate clock skew.
func NewCA(tb testing.TB) *CA {
	tb.Helper()
	return &CA{CA: testhelpers.NewCA(tb)}
}

// ServerCert mints a server-auth leaf valid for the given IP, signed by the CA.
func (ca *CA) ServerCert(tb testing.TB, ip net.IP) tls.Certificate {
	tb.Helper()
	return ca.SignLeaf(tb,
		testhelpers.WithCommonName("server"),
		testhelpers.WithIPAddresses(ip),
		testhelpers.WithExtKeyUsage(x509.ExtKeyUsageServerAuth),
	)
}

// ClientCert mints a client-auth leaf carrying spiffeID as its sole URI SAN,
// signed by the CA.
func (ca *CA) ClientCert(tb testing.TB, spiffeID string) tls.Certificate {
	tb.Helper()
	uri, err := url.Parse(spiffeID)
	if err != nil {
		tb.Fatalf("parse spiffe id %q: %v", spiffeID, err)
	}
	return ca.SignLeaf(tb,
		testhelpers.WithCommonName("client"),
		testhelpers.WithURIs(uri),
		testhelpers.WithExtKeyUsage(x509.ExtKeyUsageClientAuth),
	)
}
