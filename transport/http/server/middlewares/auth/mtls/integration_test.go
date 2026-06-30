// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls_test

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/spiffe"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/mtls"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
	authmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"
)

// makeCA returns a self-signed CA certificate and its signing key.
func makeCA(t *testing.T) (*x509.Certificate, crypto.Signer) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "e2e-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	require.NoError(t, err)
	ca, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return ca, key
}

// signLeaf signs tmpl with the CA and returns it as a usable tls.Certificate.
func signLeaf(t *testing.T, ca *x509.Certificate, caKey crypto.Signer, tmpl *x509.Certificate) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, key.Public(), caKey)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

// TestMiddlewareRealHandshake drives the middleware over a genuine mutual-TLS
// handshake: a real server requires and verifies the client certificate, so the
// VerifiedChains the middleware reads are populated by the TLS stack itself
// (not synthesized). It asserts the SPIFFE identity carried in the client
// certificate's URI SAN reaches the handler as the installed principal.
func TestMiddlewareRealHandshake(t *testing.T) {
	t.Parallel()
	ca, caKey := makeCA(t)
	pool := x509.NewCertPool()
	pool.AddCert(ca)

	serverCert := signLeaf(t, ca, caKey, &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "server"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	spiffeID := "spiffe://example.org/sa/billing"
	spiffeURI, err := url.Parse(spiffeID)
	require.NoError(t, err)
	clientCert := signLeaf(t, ca, caKey, &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "client"},
		URIs:         []*url.URL{spiffeURI},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	tlsLn := tls.NewListener(ln, &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	})

	handler := mtls.Middleware(coremtls.WithValidator(coremtls.TrustDomainValidator("example.org")))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := authmw.FromContext(r.Context()).(spiffe.ID)
			_, _ = io.WriteString(w, id.String())
		}))
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(tlsLn) }()
	t.Cleanup(func() { _ = srv.Close() })

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      pool,
		MinVersion:   tls.VersionTLS13,
	}}}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+ln.Addr().String()+"/", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, spiffeID, string(body)) // peer cert → spiffe.ID principal, end to end
}
