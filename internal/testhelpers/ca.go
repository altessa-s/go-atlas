// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

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

// CertOption mutates an x509 certificate template before it is signed. The
// same options configure both [NewCA] and [CA.SignLeaf]; each applies on top
// of the respective base template.
type CertOption func(*x509.Certificate)

// WithCommonName sets the subject common name of the certificate.
func WithCommonName(cn string) CertOption {
	return func(tmpl *x509.Certificate) { tmpl.Subject.CommonName = cn }
}

// WithSubjectKeyID pins the SubjectKeyId extension to id instead of the hash
// Go derives from the public key. Leaves signed by the CA chain to it through
// their AuthorityKeyId automatically in either case.
func WithSubjectKeyID(id []byte) CertOption {
	return func(tmpl *x509.Certificate) { tmpl.SubjectKeyId = id }
}

// WithSerial pins the certificate serial number instead of the CA's
// auto-incremented default.
func WithSerial(serial int64) CertOption {
	return func(tmpl *x509.Certificate) { tmpl.SerialNumber = big.NewInt(serial) }
}

// WithDNSNames adds DNS SANs to the certificate.
func WithDNSNames(names ...string) CertOption {
	return func(tmpl *x509.Certificate) { tmpl.DNSNames = append(tmpl.DNSNames, names...) }
}

// WithIPAddresses adds IP SANs to the certificate.
func WithIPAddresses(ips ...net.IP) CertOption {
	return func(tmpl *x509.Certificate) { tmpl.IPAddresses = append(tmpl.IPAddresses, ips...) }
}

// WithURIs adds URI SANs (e.g. SPIFFE IDs) to the certificate.
func WithURIs(uris ...*url.URL) CertOption {
	return func(tmpl *x509.Certificate) { tmpl.URIs = append(tmpl.URIs, uris...) }
}

// WithExtKeyUsage adds extended key usages to the certificate.
func WithExtKeyUsage(usages ...x509.ExtKeyUsage) CertOption {
	return func(tmpl *x509.Certificate) { tmpl.ExtKeyUsage = append(tmpl.ExtKeyUsage, usages...) }
}

// WithOCSPServers sets the OCSP responder URLs embedded in the certificate.
func WithOCSPServers(urls ...string) CertOption {
	return func(tmpl *x509.Certificate) { tmpl.OCSPServer = urls }
}

// CA is an in-memory ECDSA P-256 certificate authority for TLS and x509
// tests. It mints short-lived leaf certificates signed by one root and
// exposes a Pool that trusts that root, so both ends of a real handshake can
// be configured without touching disk. All certificates are valid for one
// hour and pre-dated by one hour to tolerate clock skew.
type CA struct {
	Cert *x509.Certificate
	Key  crypto.Signer
	Pool *x509.CertPool

	serial atomic.Int64
}

// NewCA generates a fresh in-memory CA. Options may override base template
// fields, e.g. [WithCommonName] or [WithSubjectKeyID].
func NewCA(tb testing.TB, opts ...CertOption) *CA {
	tb.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		tb.Fatalf("generate CA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	for _, opt := range opts {
		opt(tmpl)
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
	ca.serial.Store(1) // the CA itself used serial 1
	return ca
}

// SignLeaf mints a leaf certificate signed by the CA with a fresh ECDSA P-256
// key. The base template carries only an auto-incremented serial, the
// validity window, and digital-signature key usage — subject, SANs, EKUs,
// serial, and OCSP URLs come from options. The leaf's AuthorityKeyId chains
// to the CA's SubjectKeyId automatically. The returned [tls.Certificate] has
// Leaf populated, so callers needing the bare [*x509.Certificate] or the
// private key can use cert.Leaf and cert.PrivateKey.
func (ca *CA) SignLeaf(tb testing.TB, opts ...CertOption) tls.Certificate {
	tb.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		tb.Fatalf("generate leaf key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(ca.serial.Add(1)),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	for _, opt := range opts {
		opt(tmpl)
	}
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
