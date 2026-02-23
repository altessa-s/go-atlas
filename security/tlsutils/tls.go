// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsutils

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"github.com/youmark/pkcs8"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

const (
	pemTypeCertificate = "CERTIFICATE"

	// maxKeyFilePermissions is the maximum allowed file mode for private key files.
	// Key files must not be readable by group or others (i.e., at most 0600).
	maxKeyFilePermissions os.FileMode = 0o600
)

// OCSPStapler defines the interface for OCSP stapling functionality.
// OCSP stapling improves TLS handshake performance by including certificate
// revocation status in the TLS handshake, reducing client-side OCSP requests.
type OCSPStapler interface {
	// GetOCSPStaple returns the OCSP staple for the given certificate.
	// The context controls the HTTP request timeout and cancellation.
	GetOCSPStaple(ctx context.Context, cert *tls.Certificate) ([]byte, error)
	// RunRefreshAll refreshes all OCSP responses in the cache that need it.
	// This method is designed to be called periodically via an external scheduler.
	RunRefreshAll(ctx context.Context) error
}

// CloneCertificateWithOCSPStaple creates a copy of the certificate with the provided OCSP staple.
// This is useful when updating OCSP responses without modifying the original certificate.
func CloneCertificateWithOCSPStaple(cert *tls.Certificate, ocspStaple []byte) *tls.Certificate {
	return &tls.Certificate{
		Certificate:                 cert.Certificate,
		PrivateKey:                  cert.PrivateKey,
		OCSPStaple:                  ocspStaple,
		SignedCertificateTimestamps: cert.SignedCertificateTimestamps,
		Leaf:                        cert.Leaf,
	}
}

var cipherSuits = []uint16{
	// 1.3
	tls.TLS_AES_128_GCM_SHA256,
	tls.TLS_AES_256_GCM_SHA384,
	tls.TLS_CHACHA20_POLY1305_SHA256,
	// 1.2
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
}

// checkKeyFilePermissions verifies that a private key file is not accessible
// by group or others. Returns an error if permissions exceed 0600.
func checkKeyFilePermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	mode := info.Mode().Perm()
	if mode&^maxKeyFilePermissions != 0 {
		return fmt.Errorf("private key file %q has insecure permissions %04o, maximum allowed is %04o",
			path, mode, maxKeyFilePermissions)
	}
	return nil
}

// loadWithDecryption attempts to load a certificate using modern PKCS#8 decryption.
func loadWithDecryption(certPem, keyPem []byte, keyPassword string) (*tls.Certificate, error) {
	// Parse the certificate
	certBlock, _ := pem.Decode(certPem)
	if certBlock == nil || certBlock.Type != pemTypeCertificate {
		return nil, fmt.Errorf("failed to parse certificate PEM")
	}

	// Parse the private key
	keyBlock, _ := pem.Decode(keyPem)
	if keyBlock == nil {
		return nil, fmt.Errorf("failed to parse private key PEM")
	}

	var privateKey any
	var err error

	// Try to decrypt if it's encrypted PKCS#8
	if keyBlock.Type == "ENCRYPTED PRIVATE KEY" {
		// Use youmark/pkcs8 library for encrypted PKCS#8
		if keyPassword == "" {
			return nil, fmt.Errorf("password required for encrypted PKCS#8 key")
		}
		privateKey, err = pkcs8.ParsePKCS8PrivateKey(keyBlock.Bytes, []byte(keyPassword))
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "decrypt PKCS#8 private key")
		}
	} else {
		// Try parsing based on the block type
		switch keyBlock.Type {
		case "PRIVATE KEY":
			// PKCS#8 unencrypted
			privateKey, err = x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
		case "RSA PRIVATE KEY":
			// PKCS#1 RSA
			privateKey, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
		case "EC PRIVATE KEY":
			// SEC1 EC
			privateKey, err = x509.ParseECPrivateKey(keyBlock.Bytes)
		default:
			return nil, fmt.Errorf("unsupported private key type: %s", keyBlock.Type)
		}

		if err != nil {
			return nil, coreerrs.WrapOperation(err, "parse private key")
		}
	}

	// Create the certificate
	cert := &tls.Certificate{
		Certificate: [][]byte{certBlock.Bytes},
		PrivateKey:  privateKey,
	}

	// Parse the leaf certificate
	cert.Leaf, err = x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "parse certificate")
	}

	return cert, nil
}

// LoadFromFile loads a certificate and private key from separate files.
// Supports multiple private key formats including PKCS#8, PKCS#1 (RSA), SEC1 (EC),
// and legacy-encrypted PEM formats. The keyPassword parameter is required for
// encrypted private keys and ignored for unencrypted keys.
//
// Example:
//
//	cert, err := tlsutils.LoadFromFile("server.key", "server.crt", "password")
//	if err != nil {
//		log.Fatal(err)
//	}
func LoadFromFile(keyPath, certPath, keyPassword string) (*tls.Certificate, error) {
	// Read certificate file with context check
	certPem, err := readFile(certPath)
	if err != nil {
		return nil, err
	}

	// Verify key file permissions before reading
	if permErr := checkKeyFilePermissions(keyPath); permErr != nil {
		return nil, permErr
	}

	// Read key file with context check
	keyPem, err := readFile(keyPath)
	if err != nil {
		return nil, err
	}

	// Try a modern approach first for encrypted keys
	if keyPassword != "" {
		cert, err := loadWithDecryption(certPem, keyPem, keyPassword)
		if err == nil {
			return cert, nil
		}
		// Fall back to legacy method if a modern approach fails
	}

	keyPem = append(keyPem, '\n')
	keyPem = append(keyPem, certPem...)

	return certFromBytes(keyPem, keyPassword)
}

// LoadFromConcatenatedFile loads a certificate and private key from a single file.
// The file should contain both the certificate and private key in PEM format.
// The keyPassword parameter is required for encrypted private keys.
//
// Example:
//
//	cert, err := tlsutils.LoadFromConcatenatedFile("server.pem", "password")
//	if err != nil {
//		log.Fatal(err)
//	}
func LoadFromConcatenatedFile(certKeyFilePath, keyPassword string) (*tls.Certificate, error) {
	// Verify key file permissions before reading
	if err := checkKeyFilePermissions(certKeyFilePath); err != nil {
		return nil, err
	}

	certPem, err := readFile(certKeyFilePath)
	if err != nil {
		return nil, err
	}

	// Try to find separate cert and key blocks for a modern approach
	if keyPassword != "" {
		var certData, keyData []byte
		remaining := certPem

		for len(remaining) > 0 {
			var block *pem.Block
			block, rest := pem.Decode(remaining)
			if block == nil {
				break
			}

			if block.Type == pemTypeCertificate {
				certData = pem.EncodeToMemory(block)
			} else if strings.HasSuffix(block.Type, "PRIVATE KEY") {
				keyData = pem.EncodeToMemory(block)
			}
			remaining = rest
		}

		if certData != nil && keyData != nil {
			cert, err := loadWithDecryption(certData, keyData, keyPassword)
			if err == nil {
				return cert, nil
			}
		}
	}

	return certFromBytes(certPem, keyPassword)
}

// DefaultTLSConfig returns a secure default TLS configuration.
// The configuration enforces TLS 1.2+ and uses modern cipher suites.
// Suitable for most server applications requiring secure connections.
//
// Example:
//
//	config := tlsutils.DefaultTLSConfig()
//	config.Certificates = []tls.Certificate{cert}
//	server := &http.Server{TLSConfig: config}
func DefaultTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		CipherSuites: cipherSuits,
	}
}

// DefaultClientTLSConfig returns a secure default TLS configuration for client connections.
// The configuration enforces TLS 1.2+ and sets the server name for certificate validation.
// The serverName parameter is used for SNI and certificate verification.
//
// Example:
//
//	config := tlsutils.DefaultClientTLSConfig("example.com")
//	conn, err := tls.Dial("tcp", "example.com:443", config)
func DefaultClientTLSConfig(serverName string) *tls.Config {
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		CipherSuites: cipherSuits,
		ServerName:   serverName,
	}
}

func certFromBytes(data []byte, keyPassword string) (*tls.Certificate, error) {
	var (
		currentBlock     *pem.Block
		certBlock        []byte
		certDecodedBlock []byte
		keyBlock         []byte
		start            = 0
	)

	remaining := data
	for {
		currentBlock, remaining = pem.Decode(remaining)
		if currentBlock == nil {
			break
		}

		if currentBlock.Type == pemTypeCertificate {
			certBlock = data[start : len(data)-len(remaining)]
			certDecodedBlock = currentBlock.Bytes
			start += len(certBlock)
			continue
		}

		if !strings.HasSuffix(currentBlock.Type, "PRIVATE KEY") {
			continue
		}

		// Handle encrypted private keys
		if keyPassword != "" && currentBlock.Headers != nil {
			// Check if the PEM block is encrypted by looking for the DEK-Info header
			if _, ok := currentBlock.Headers["DEK-Info"]; ok {
				// Deprecated but required for legacy PEM encryption (RFC 1423).
				// Modern PKCS#8 encryption is handled by loadWithDecryption.
				if x509.IsEncryptedPEMBlock(currentBlock) { //nolint:staticcheck // SA1019: required for legacy PEM support
					buf, err := x509.DecryptPEMBlock(currentBlock, []byte(keyPassword)) //nolint:staticcheck // SA1019: required for legacy PEM support
					if err != nil {
						return nil, coreerrs.WrapOperation(err, "decrypt private key")
					}

					var encoded bytes.Buffer
					if err := pem.Encode(&encoded, &pem.Block{Type: currentBlock.Type, Bytes: buf}); err != nil {
						return nil, err
					}

					keyBlock = encoded.Bytes()
					start = len(data) - len(remaining)
					continue
				}
			}
		}

		keyBlock = data[start : len(data)-len(remaining)]
		start += len(keyBlock)
	}

	cert, err := tls.X509KeyPair(certBlock, keyBlock)
	if err != nil {
		return nil, err
	}

	// The documentation for the tls.X509KeyPair indicates that the Leaf certificate is not
	// retained.
	if _, err := x509.ParseCertificate(certDecodedBlock); err != nil {
		return nil, err
	}
	return &cert, nil
}

// BuildCAPool builds a certificate pool from CA certificate files.
// If a system is true, starts with the system's certificate pool.
// Additional CA certificates from caPath files are appended to the pool.
// Returns nil if no valid certificates are found.
//
// Example:
//
//	pool, err := tlsutils.BuildCAPool(true, "ca1.crt", "ca2.crt")
//	if err != nil {
//		log.Fatal(err)
//	}
//	config.RootCAs = pool
func BuildCAPool(system bool, caPath ...string) (*x509.CertPool, error) {
	var (
		pool *x509.CertPool
		err  error
	)

	if system {
		pool, err = x509.SystemCertPool()
		if err != nil {
			return nil, err
		}
	}

	if pool == nil {
		pool = x509.NewCertPool()
	}

	for _, path := range caPath {
		caPem, err := readFile(path)
		if err != nil {
			return nil, err
		}

		if !pool.AppendCertsFromPEM(caPem) {
			return nil, fmt.Errorf("failed to parse any certificates from CA file %s", path)
		}
	}

	return pool, nil
}

// readFile reads a file.
// #nosec G304 -- path comes from trusted configuration, not user input
func readFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return data, nil
}
