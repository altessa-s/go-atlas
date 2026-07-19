// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spiffe_test

import (
	"crypto"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/spiffe/go-spiffe/v2/bundle/x509bundle"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/security/tlsutils/spiffe"
)

// staticSource is a [spiffe.Source] serving a fixed SVID and bundle — the
// in-memory stand-in for a Workload API source in the handshake tests.
type staticSource struct {
	svid   *x509svid.SVID
	bundle *x509bundle.Bundle
}

func (s *staticSource) GetX509SVID() (*x509svid.SVID, error) { return s.svid, nil }

func (s *staticSource) GetX509BundleForTrustDomain(spiffeid.TrustDomain) (*x509bundle.Bundle, error) {
	return s.bundle, nil
}

func (s *staticSource) Close() error { return nil }

// makeSVID mints a CA-signed SVID whose sole URI SAN is the given SPIFFE ID.
func makeSVID(t *testing.T, ca *testhelpers.CA, idStr string) *x509svid.SVID {
	t.Helper()
	id := spiffeid.RequireFromString(idStr)
	uri, err := url.Parse(id.String())
	require.NoError(t, err)
	cert := ca.SignLeaf(t,
		testhelpers.WithURIs(uri),
		testhelpers.WithExtKeyUsage(x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth),
	)
	signer, ok := cert.PrivateKey.(crypto.Signer)
	require.True(t, ok)
	return &x509svid.SVID{ID: id, Certificates: []*x509.Certificate{cert.Leaf}, PrivateKey: signer}
}

// startServer runs an mTLS HTTP server with the given provider on a random port
// and returns its address.
func startServer(t *testing.T, provider *spiffe.Provider) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := &http.Server{
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }),
		TLSConfig:         provider.MTLSServerConfig(),
		ReadHeaderTimeout: time.Second,
	}
	go func() { _ = srv.ServeTLS(ln, "", "") }()
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().String()
}

// TestMTLSHandshake exercises a real handshake: a client provider dials a server
// provider, both presenting CA-signed SVIDs, and the trust-domain authorizers
// accept each other.
func TestMTLSHandshake(t *testing.T) {
	t.Parallel()
	td := spiffeid.RequireTrustDomainFromString("example.org")
	ca := testhelpers.NewCA(t)
	bundle := x509bundle.FromX509Authorities(td, []*x509.Certificate{ca.Cert})

	server, err := spiffe.NewProvider(
		&staticSource{makeSVID(t, ca, "spiffe://example.org/server"), bundle},
		tlsconfig.AuthorizeMemberOf(td),
	)
	require.NoError(t, err)
	client, err := spiffe.NewProvider(
		&staticSource{makeSVID(t, ca, "spiffe://example.org/client"), bundle},
		tlsconfig.AuthorizeMemberOf(td),
	)
	require.NoError(t, err)

	addr := startServer(t, server)
	httpClient := &http.Client{Transport: client.HTTPTransport(), Timeout: 5 * time.Second}

	resp, err := httpClient.Get("https://" + addr)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "ok", string(body))
}

// TestMTLSHandshakeRejectsUnauthorizedPeer confirms the server's authorizer
// fails the handshake when the client's SPIFFE ID is not allowed, even though
// the certificate chains to a trusted CA.
func TestMTLSHandshakeRejectsUnauthorizedPeer(t *testing.T) {
	t.Parallel()
	td := spiffeid.RequireTrustDomainFromString("example.org")
	ca := testhelpers.NewCA(t)
	bundle := x509bundle.FromX509Authorities(td, []*x509.Certificate{ca.Cert})

	// Server only authorizes a specific ID the client does not have.
	server, err := spiffe.NewProvider(
		&staticSource{makeSVID(t, ca, "spiffe://example.org/server"), bundle},
		tlsconfig.AuthorizeID(spiffeid.RequireFromString("spiffe://example.org/allowed")),
	)
	require.NoError(t, err)
	client, err := spiffe.NewProvider(
		&staticSource{makeSVID(t, ca, "spiffe://example.org/client"), bundle},
		tlsconfig.AuthorizeMemberOf(td),
	)
	require.NoError(t, err)

	addr := startServer(t, server)
	httpClient := &http.Client{Transport: client.HTTPTransport(), Timeout: 5 * time.Second}

	resp, err := httpClient.Get("https://" + addr)
	if err == nil {
		_ = resp.Body.Close()
	}
	require.Error(t, err)
}
