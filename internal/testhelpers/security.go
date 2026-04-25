// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// GenerateRSAKey generates an RSA private key of the given bit size for testing.
func GenerateRSAKey(tb testing.TB, bits int) *rsa.PrivateKey {
	tb.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		tb.Fatalf("failed to generate RSA key: %v", err)
	}
	return key
}

// GenerateRSAKeyPEM generates an RSA private key and returns it as PKCS#8
// PEM-encoded bytes.
func GenerateRSAKeyPEM(tb testing.TB, bits int) []byte {
	tb.Helper()
	key := GenerateRSAKey(tb, bits)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		tb.Fatalf("failed to marshal private key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

// SelfSignedCert generates a CA-signed leaf TLS certificate for testing with no
// OCSP servers configured. It delegates to [SelfSignedCertWithOCSP].
func SelfSignedCert(tb testing.TB) (certPEM, keyPEM []byte, cert tls.Certificate) {
	tb.Helper()
	return SelfSignedCertWithOCSP(tb, nil)
}

// SelfSignedCertWithOCSP generates a CA-signed leaf TLS certificate with optional
// OCSP server URLs. It creates a short-lived CA and a leaf certificate valid for
// "localhost". Both certificates are pre-dated by 1 hour (to tolerate clock skew)
// and valid for 24 hours. The returned [tls.Certificate] contains the full chain
// (leaf + CA).
func SelfSignedCertWithOCSP(tb testing.TB, ocspServers []string) (certPEM, keyPEM []byte, cert tls.Certificate) {
	tb.Helper()

	// Generate CA key and certificate
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		tb.Fatalf("failed to generate CA key: %v", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour), //nolint:mnd // test constant
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		tb.Fatalf("failed to create CA certificate: %v", err)
	}

	// Generate leaf key and certificate
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		tb.Fatalf("failed to generate leaf key: %v", err)
	}

	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2), //nolint:mnd // test constant
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour), //nolint:mnd // test constant
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		OCSPServer:   ocspServers,
	}

	leafCertDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caTemplate, &leafKey.PublicKey, caKey)
	if err != nil {
		tb.Fatalf("failed to create leaf certificate: %v", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafCertDER})
	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})

	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		tb.Fatalf("failed to marshal leaf key: %v", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: leafKeyDER})

	// Build tls.Certificate with full chain
	cert = tls.Certificate{
		Certificate: [][]byte{leafCertDER, caCertDER},
		PrivateKey:  leafKey,
	}
	cert.Leaf, _ = x509.ParseCertificate(leafCertDER) //nolint:errcheck // cert was just created; parse cannot fail

	// Append CA cert PEM for chain verification in file-based loading
	_ = caCertPEM // CA cert PEM available if needed for chain

	return certPEM, keyPEM, cert
}

// WriteTempCertFiles writes cert and key PEM data to temporary files (0o600 permissions)
// inside tb.TempDir and returns their paths. The files are cleaned up automatically.
func WriteTempCertFiles(tb testing.TB, certPEM, keyPEM []byte) (certPath, keyPath string) {
	tb.Helper()
	dir := tb.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil { //nolint:mnd // file permission
		tb.Fatalf("failed to write cert file: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil { //nolint:mnd // file permission
		tb.Fatalf("failed to write key file: %v", err)
	}
	return
}
