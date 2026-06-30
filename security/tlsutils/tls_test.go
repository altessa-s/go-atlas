// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsutils_test

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/security/tlsutils"
)

// stubClientCert is a clientCertificateSource that always returns the same
// certificate and records whether it was asked.
type stubClientCert struct {
	cert   tls.Certificate
	called bool
}

func (s *stubClientCert) GetClientCertificate(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
	s.called = true
	return &s.cert, nil
}

func TestLoadFromBytes_Valid(t *testing.T) {
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)

	cert, err := tlsutils.LoadFromBytes(certPEM, keyPEM, "")
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.NotEmpty(t, cert.Certificate)
}

func TestLoadFromBytes_InvalidCert(t *testing.T) {
	_, err := tlsutils.LoadFromBytes([]byte("not-a-cert"), []byte("not-a-key"), "")
	require.Error(t, err)
}

func TestLoadFromBytes_NilInputs(t *testing.T) {
	_, err := tlsutils.LoadFromBytes(nil, nil, "")
	require.Error(t, err)
}

func TestDefaultTLSConfig(t *testing.T) {
	config := tlsutils.DefaultTLSConfig()
	require.Equal(t, uint16(tls.VersionTLS12), config.MinVersion)
	require.NotEmpty(t, config.CipherSuites)
}

func TestDefaultClientTLSConfig(t *testing.T) {
	config := tlsutils.DefaultClientTLSConfig("example.com")
	require.Equal(t, uint16(tls.VersionTLS12), config.MinVersion)
	require.Equal(t, "example.com", config.ServerName)
}

func TestClientTLSConfig(t *testing.T) {
	_, _, cert := testhelpers.SelfSignedCert(t)
	src := &stubClientCert{cert: cert}

	config := tlsutils.ClientTLSConfig(src, "peer.internal")
	require.Equal(t, uint16(tls.VersionTLS13), config.MinVersion)
	require.Equal(t, "peer.internal", config.ServerName)
	require.NotNil(t, config.GetClientCertificate)

	got, err := config.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.NoError(t, err)
	require.True(t, src.called)
	require.Equal(t, cert.Certificate, got.Certificate)
}

func TestDialContext(t *testing.T) {
	_, _, cert := testhelpers.SelfSignedCert(t)
	src := &stubClientCert{cert: cert}

	// A TLS server whose certificate is signed by an untrusted (test) CA. The
	// strict client config built by DialContext verifies against the system
	// roots, so the handshake must fail at server-certificate verification —
	// proving DialContext performed a real strict-TLS handshake.
	srv := httptest.NewTLSServer(nil)
	t.Cleanup(srv.Close)

	conn, err := tlsutils.DialContext(t.Context(), "tcp", srv.Listener.Addr().String(), src, "")
	if conn != nil {
		_ = conn.Close()
	}
	var verifyErr *tls.CertificateVerificationError
	require.ErrorAs(t, err, &verifyErr)
}

func TestCloneCertificateWithOCSPStaple(t *testing.T) {
	original := &tls.Certificate{
		Certificate:                 [][]byte{{0x01, 0x02}},
		PrivateKey:                  "test-key",
		SignedCertificateTimestamps: [][]byte{{0x04, 0x05}},
	}
	staple := []byte{0x06, 0x07, 0x08}

	cloned := tlsutils.CloneCertificateWithOCSPStaple(original, staple)

	require.Len(t, cloned.OCSPStaple, len(staple))
	require.Len(t, cloned.Certificate, len(original.Certificate))
	require.Equal(t, original.PrivateKey, cloned.PrivateKey)
}

func TestBuildCAPool_SystemOnly(t *testing.T) {
	pool, err := tlsutils.BuildCAPool(true)
	if err != nil {
		t.Skipf("system cert pool not available: %v", err)
	}
	require.NotNil(t, pool)
}

func TestBuildCAPool_InvalidFile(t *testing.T) {
	_, err := tlsutils.BuildCAPool(false, "/nonexistent")
	require.Error(t, err)
}

func TestLoadFromFile_InvalidPaths(t *testing.T) {
	_, err := tlsutils.LoadFromFile("/nonexistent/key", "/nonexistent/cert", "")
	require.Error(t, err)
}

func TestLoadFromConcatenatedFile_InvalidPath(t *testing.T) {
	_, err := tlsutils.LoadFromConcatenatedFile("/nonexistent/file", "")
	require.Error(t, err)
}
