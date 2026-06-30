// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtlstest

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

// CA is a self-signed certificate authority for mutual-TLS integration tests.
// It mints short-lived server and client leaf certificates signed by the same
// root, and exposes a Pool that trusts that root — enough to stand up a real
// handshake on both ends (server ClientCAs, client RootCAs).
type CA struct {
	Cert *x509.Certificate
	Key  crypto.Signer
	Pool *x509.CertPool

	serial atomic.Int64
}

// NewCA generates a fresh in-memory CA. The certificate is valid for one hour,
// pre-dated to tolerate clock skew.
func NewCA(tb testing.TB) *CA {
	tb.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		tb.Fatalf("generate CA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "mtlstest-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		tb.Fatalf("create CA certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		tb.Fatalf("parse CA certificate: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	ca := &CA{Cert: cert, Key: key, Pool: pool}
	ca.serial.Store(1) // CA itself used serial 1.
	return ca
}

// ServerCert mints a server-auth leaf valid for the given IP, signed by the CA.
func (ca *CA) ServerCert(tb testing.TB, ip net.IP) tls.Certificate {
	tb.Helper()
	return ca.sign(tb, &x509.Certificate{
		Subject:     pkix.Name{CommonName: "server"},
		IPAddresses: []net.IP{ip},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
}

// ClientCert mints a client-auth leaf carrying spiffeID as its sole URI SAN,
// signed by the CA.
func (ca *CA) ClientCert(tb testing.TB, spiffeID string) tls.Certificate {
	tb.Helper()
	uri, err := url.Parse(spiffeID)
	if err != nil {
		tb.Fatalf("parse spiffe id %q: %v", spiffeID, err)
	}
	return ca.sign(tb, &x509.Certificate{
		Subject:     pkix.Name{CommonName: "client"},
		URIs:        []*url.URL{uri},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
}

// sign fills the common leaf fields, signs tmpl with the CA, and returns a
// ready-to-use tls.Certificate.
func (ca *CA) sign(tb testing.TB, tmpl *x509.Certificate) tls.Certificate {
	tb.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		tb.Fatalf("generate leaf key: %v", err)
	}
	tmpl.SerialNumber = big.NewInt(ca.serial.Add(1))
	tmpl.NotBefore = time.Now().Add(-time.Hour)
	tmpl.NotAfter = time.Now().Add(time.Hour)
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Cert, key.Public(), ca.Key)
	if err != nil {
		tb.Fatalf("create leaf certificate: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		tb.Fatalf("parse leaf certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}
