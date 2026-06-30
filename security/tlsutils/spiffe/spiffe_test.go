// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spiffe_test

import (
	"crypto/tls"
	"errors"
	"testing"

	"github.com/spiffe/go-spiffe/v2/bundle/x509bundle"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/tlsutils/spiffe"
)

// fakeSource is a minimal [spiffe.Source]. The TLS configs only call into it
// during a handshake, so returning errors here is enough to exercise the wiring
// without standing up a Workload API server.
type fakeSource struct{ closed bool }

func (f *fakeSource) GetX509SVID() (*x509svid.SVID, error) { return nil, errors.New("no svid") }

func (f *fakeSource) GetX509BundleForTrustDomain(spiffeid.TrustDomain) (*x509bundle.Bundle, error) {
	return nil, errors.New("no bundle")
}

func (f *fakeSource) Close() error { f.closed = true; return nil }

func anyAuthorizer() tlsconfig.Authorizer { return tlsconfig.AuthorizeAny() }

func TestNewProviderRejectsNilDeps(t *testing.T) {
	t.Parallel()

	_, err := spiffe.NewProvider(nil, anyAuthorizer())
	require.ErrorIs(t, err, spiffe.ErrNoSource)

	_, err = spiffe.NewProvider(&fakeSource{}, nil)
	require.ErrorIs(t, err, spiffe.ErrNoAuthorizer)
}

func TestNewRequiresAuthorizer(t *testing.T) {
	t.Parallel()
	// No authorizer: fails closed before any Workload API dial.
	_, err := spiffe.New(t.Context())
	require.ErrorIs(t, err, spiffe.ErrNoAuthorizer)
}

func TestMTLSServerConfig(t *testing.T) {
	t.Parallel()
	p, err := spiffe.NewProvider(&fakeSource{}, anyAuthorizer())
	require.NoError(t, err)

	cfg := p.MTLSServerConfig()
	require.NotNil(t, cfg)
	require.Equal(t, tls.RequireAnyClientCert, cfg.ClientAuth)
	require.NotNil(t, cfg.GetCertificate)
	require.NotNil(t, cfg.VerifyPeerCertificate)
}

func TestMTLSClientConfig(t *testing.T) {
	t.Parallel()
	p, err := spiffe.NewProvider(&fakeSource{}, anyAuthorizer())
	require.NoError(t, err)

	cfg := p.MTLSClientConfig()
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.GetClientCertificate)
	require.NotNil(t, cfg.VerifyPeerCertificate)
	// go-spiffe disables the stdlib verifier and runs its own.
	require.True(t, cfg.InsecureSkipVerify)
}

func TestHTTPTransport(t *testing.T) {
	t.Parallel()
	p, err := spiffe.NewProvider(&fakeSource{}, anyAuthorizer())
	require.NoError(t, err)

	tr := p.HTTPTransport()
	require.NotNil(t, tr)
	require.NotNil(t, tr.DialTLSContext)
}

func TestGRPCCredentials(t *testing.T) {
	t.Parallel()
	p, err := spiffe.NewProvider(&fakeSource{}, anyAuthorizer())
	require.NoError(t, err)

	server := p.ServerCredentials()
	require.NotNil(t, server)
	require.Equal(t, "tls", server.Info().SecurityProtocol)

	client := p.ClientCredentials()
	require.NotNil(t, client)
	require.Equal(t, "tls", client.Info().SecurityProtocol)
}

func TestClose(t *testing.T) {
	t.Parallel()
	src := &fakeSource{}
	p, err := spiffe.NewProvider(src, anyAuthorizer())
	require.NoError(t, err)
	require.NoError(t, p.Close())
	require.True(t, src.closed)
}
