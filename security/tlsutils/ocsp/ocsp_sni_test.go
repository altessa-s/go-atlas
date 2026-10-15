// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ocsp

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

// handshakeNoSNI runs a TLS handshake against server over an in-memory pipe
// from a client that sends no server_name extension. It returns the client's
// connection state and the client-side handshake error.
func handshakeNoSNI(t *testing.T, server, client *tls.Config) (tls.ConnectionState, error) {
	t.Helper()
	cConn, sConn := net.Pipe()
	srv := tls.Server(sConn, server)
	cli := tls.Client(cConn, client)
	srvDone := make(chan struct{})
	go func() {
		defer close(srvDone)
		_ = srv.HandshakeContext(t.Context())
		// Close the raw pipe, not the TLS conn: a TLS close_notify would block on
		// the synchronous pipe. This also unblocks the client on a server abort.
		_ = sConn.Close()
	}()

	err := cli.HandshakeContext(t.Context())
	state := cli.ConnectionState()
	_ = cConn.Close()
	<-srvDone
	return state, err
}

// noSNIClient is a client config that cannot send SNI: ServerName is empty, so
// crypto/tls omits the extension (verification is skipped for the same reason).
func noSNIClient() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12} //nolint:gosec // test: no SNI means no name to verify
}

func TestStapleOCSPToConfig_EnforcesWithoutSNI(t *testing.T) {
	t.Parallel()

	t.Run("hard mode aborts a handshake without SNI", func(t *testing.T) {
		t.Parallel()
		_, _, cert := testhelpers.SelfSignedCert(t)
		cfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
		require.NoError(t, StapleOCSPToConfig(cfg, &failingStapler{
			err:         errors.New("responder unreachable"),
			failureMode: FailureModeHard,
		}))

		_, err := handshakeNoSNI(t, cfg, noSNIClient())
		require.Error(t, err, "hard-fail must not be bypassed by omitting SNI")
	})

	t.Run("soft mode serves the static certificate without SNI", func(t *testing.T) {
		t.Parallel()
		_, _, cert := testhelpers.SelfSignedCert(t)
		cfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
		require.NoError(t, StapleOCSPToConfig(cfg, &failingStapler{
			err:         errors.New("responder unreachable"),
			failureMode: FailureModeSoft,
		}))

		state, err := handshakeNoSNI(t, cfg, noSNIClient())
		require.NoError(t, err)
		require.Equal(t, cert.Certificate[0], state.PeerCertificates[0].Raw)
		require.Empty(t, state.OCSPResponse)
	})

	t.Run("hard mode staples a good response without SNI", func(t *testing.T) {
		t.Parallel()
		ca := testhelpers.NewCA(t)
		cert := chainFor(t, ca, testResponder(t, ca, responderSpec{
			thisUpdate: time.Now().Add(-time.Minute),
			nextUpdate: time.Now().Add(time.Hour),
		}))
		cfg := &tls.Config{Certificates: []tls.Certificate{*cert}, MinVersion: tls.VersionTLS12}
		require.NoError(t, StapleOCSPToConfig(cfg, NewOCSPStapler(WithFailureMode(FailureModeHard))))

		state, err := handshakeNoSNI(t, cfg, noSNIClient())
		require.NoError(t, err)
		require.NotEmpty(t, state.OCSPResponse)
	})
}

// rsaCert issues a self-signed RSA server certificate.
func rsaCert(t *testing.T) tls.Certificate {
	t.Helper()
	key := testhelpers.GenerateRSAKey(t, 2048)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(7),
		Subject:      pkix.Name{CommonName: "rsa"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func TestStapleOCSPToConfig_PreservesStaticSelection(t *testing.T) {
	t.Parallel()
	_, _, ecdsaCert := testhelpers.SelfSignedCert(t)
	rsa := rsaCert(t)
	cfg := &tls.Config{Certificates: []tls.Certificate{ecdsaCert, rsa}, MinVersion: tls.VersionTLS12}
	require.NoError(t, StapleOCSPToConfig(cfg, &failingStapler{
		err:         errors.New("responder unreachable"),
		failureMode: FailureModeSoft,
	}))

	// A TLS 1.2 client offering only ECDHE-RSA suites cannot use the first
	// (ECDSA) certificate; crypto/tls would pick the RSA one, and so must the
	// enforcing callback.
	client := noSNIClient()
	client.MaxVersion = tls.VersionTLS12
	client.CipherSuites = []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}

	state, err := handshakeNoSNI(t, cfg, client)
	require.NoError(t, err)
	require.Equal(t, rsa.Certificate[0], state.PeerCertificates[0].Raw)
}

func TestStapleOCSPToConfig_KeepsStaticClientCertificates(t *testing.T) {
	t.Parallel()
	_, _, cert := testhelpers.SelfSignedCert(t)
	cfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	require.NoError(t, StapleOCSPToConfig(cfg, &failingStapler{
		err:         errors.New("responder unreachable"),
		failureMode: FailureModeHard,
	}))
	require.NotNil(t, cfg.GetClientCertificate, "static certificates must stay available to the client side")

	got, err := cfg.GetClientCertificate(&tls.CertificateRequestInfo{
		SignatureSchemes: []tls.SignatureScheme{tls.ECDSAWithP256AndSHA256},
		Version:          tls.VersionTLS13,
	})
	require.NoError(t, err)
	require.Equal(t, cert.Certificate, got.Certificate, "client static certificates are not stapled (unchanged behavior)")
}

func TestStapleOCSPToConfig_StaticClientCertificateNoMatch(t *testing.T) {
	t.Parallel()
	_, _, cert := testhelpers.SelfSignedCert(t)
	cfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	require.NoError(t, StapleOCSPToConfig(cfg, NewOCSPStapler()))

	// An ECDSA certificate cannot satisfy an RSA-only request: crypto/tls sends
	// an empty certificate, and so does the installed callback.
	got, err := cfg.GetClientCertificate(&tls.CertificateRequestInfo{
		SignatureSchemes: []tls.SignatureScheme{tls.PSSWithSHA256},
		Version:          tls.VersionTLS13,
	})
	require.NoError(t, err)
	require.Empty(t, got.Certificate)
}

func TestSelectServerCertificate(t *testing.T) {
	t.Parallel()
	_, _, ecdsaCert := testhelpers.SelfSignedCert(t)
	rsa := rsaCert(t)
	statics := []tls.Certificate{ecdsaCert, rsa}

	t.Run("callback returning nil falls back to the static certificates", func(t *testing.T) {
		t.Parallel()
		getCert := func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return nil, nil } //nolint:nilnil // crypto/tls contract
		got, err := selectServerCertificate(&tls.ClientHelloInfo{ServerName: "x"}, getCert, statics[:1], nil)
		require.NoError(t, err)
		require.Equal(t, ecdsaCert.Certificate, got.Certificate)
	})

	t.Run("callback is skipped without SNI when static certificates exist", func(t *testing.T) {
		t.Parallel()
		getCert := func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return &rsa, nil }
		got, err := selectServerCertificate(&tls.ClientHelloInfo{}, getCert, statics[:1], nil)
		require.NoError(t, err)
		require.Equal(t, ecdsaCert.Certificate, got.Certificate)
	})

	t.Run("name map matches a lowercased wildcard", func(t *testing.T) {
		t.Parallel()
		names := coremaps.NewImmutableMap(map[string]*tls.Certificate{"*.example.org": &statics[1]})
		got, err := selectServerCertificate(&tls.ClientHelloInfo{ServerName: "API.Example.org"}, nil, statics, names)
		require.NoError(t, err)
		require.Equal(t, rsa.Certificate, got.Certificate)
	})

	t.Run("no certificates", func(t *testing.T) {
		t.Parallel()
		_, err := selectServerCertificate(&tls.ClientHelloInfo{}, nil, nil, nil)
		require.Error(t, err)
	})
}
