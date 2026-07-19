// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spiffe

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/spiffe/go-spiffe/v2/bundle/x509bundle"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"

	"google.golang.org/grpc/credentials"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

var (
	// ErrNoAuthorizer indicates no peer authorizer was configured. The provider
	// is fail-closed: without an authorizer it refuses to build, rather than
	// accepting any peer SPIFFE ID.
	ErrNoAuthorizer = errors.New("spiffe: peer authorizer required")
	// ErrNoSource indicates a nil X.509 source was passed to [NewProvider].
	ErrNoSource = errors.New("spiffe: nil X509 source")
)

// Source supplies the rotating X.509-SVID and trust bundle that back the TLS
// configurations. [workloadapi.X509Source] satisfies it: it streams fresh SVIDs
// and bundles from the SPIFFE Workload API and keeps them current as they
// rotate. The interface is the seam — callers that already own a source (or a
// test double) can wrap it with [NewProvider].
type Source interface {
	x509svid.Source
	x509bundle.Source
	io.Closer
}

// Provider builds rotating mutual-TLS configurations from a SPIFFE [Source]. The
// returned *tls.Config values pull the current SVID and bundle on every
// handshake, so a long-running server or client keeps presenting and trusting
// the right material across SVID rotations without a restart. Safe for
// concurrent use.
type Provider struct {
	source     Source
	authorizer tlsconfig.Authorizer
}

// New builds a Provider backed by a SPIFFE Workload API X.509 source. It blocks
// until the first SVID has been received, then returns. The provider must be
// closed when no longer in use to release the Workload API connection.
//
// An authorizer is mandatory ([WithAuthorizer]); without it New fails with
// [ErrNoAuthorizer] rather than trusting any peer. When the socket path is
// unset the standard SPIFFE_ENDPOINT_SOCKET environment variable selects the
// Workload API endpoint.
func New(ctx context.Context, opts ...Option) (*Provider, error) {
	o := newOptions(opts...)
	if o.authorizer == nil {
		return nil, ErrNoAuthorizer
	}
	var clientOpts []workloadapi.X509SourceOption
	clientOpts = coreslices.AppendIf[workloadapi.X509SourceOption](clientOpts, o.socketPath != "",
		workloadapi.WithClientOptions(workloadapi.WithAddr(o.socketPath)))
	src, err := workloadapi.NewX509Source(ctx, clientOpts...)
	if err != nil {
		return nil, coreerrs.Wrap(err, "spiffe: create X509 source")
	}
	return &Provider{source: src, authorizer: o.authorizer}, nil
}

// NewProvider wraps an existing [Source] (for example a [workloadapi.X509Source]
// the caller built and owns, or a test double). The authorizer is mandatory.
func NewProvider(source Source, authorizer tlsconfig.Authorizer) (*Provider, error) {
	if source == nil {
		return nil, ErrNoSource
	}
	if authorizer == nil {
		return nil, ErrNoAuthorizer
	}
	return &Provider{source: source, authorizer: authorizer}, nil
}

// MTLSServerConfig returns a *tls.Config for a mutual-TLS server: it presents
// the current SVID, requires a client certificate, and verifies the peer
// against the trust bundle and the configured authorizer.
func (p *Provider) MTLSServerConfig() *tls.Config {
	return tlsconfig.MTLSServerConfig(p.source, p.source, p.authorizer)
}

// MTLSClientConfig returns a *tls.Config for a mutual-TLS client: it presents
// the current SVID and verifies the server against the trust bundle and the
// configured authorizer.
func (p *Provider) MTLSClientConfig() *tls.Config {
	return tlsconfig.MTLSClientConfig(p.source, p.source, p.authorizer)
}

// DialContext establishes a mutual-TLS connection to addr, presenting the
// current SVID and verifying the server against the trust bundle and authorizer.
// Server identity is checked by SPIFFE ID — addr's host is used only to reach
// the peer, not to match a DNS name. It is a [net.Dialer]-shaped helper for
// wiring into clients (gRPC dialers, [http.Transport.DialTLSContext]).
func (p *Provider) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	d := tls.Dialer{Config: p.MTLSClientConfig()}
	return d.DialContext(ctx, network, addr)
}

// HTTPTransport returns an [http.Transport] (cloned from [http.DefaultTransport])
// whose TLS dial uses [Provider.DialContext], so outbound HTTPS requests run over
// SPIFFE mutual TLS. Wrap it in an [http.Client].
func (p *Provider) HTTPTransport() *http.Transport {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{}
	}
	t := base.Clone()
	t.DialTLSContext = p.DialContext
	return t
}

// ServerCredentials returns gRPC transport credentials for a mutual-TLS server,
// backed by [Provider.MTLSServerConfig]. Pass them to grpc.NewServer through
// grpc.Creds (or the framework server builder); the presented SVID rotates on
// every handshake like the HTTP server config.
func (p *Provider) ServerCredentials() credentials.TransportCredentials {
	return credentials.NewTLS(p.MTLSServerConfig())
}

// ClientCredentials returns gRPC transport credentials for a mutual-TLS client,
// backed by [Provider.MTLSClientConfig]. Pass them to grpc.NewClient through
// grpc.WithTransportCredentials. Server identity is verified by SPIFFE ID, not
// the dial target's host.
func (p *Provider) ClientCredentials() credentials.TransportCredentials {
	return credentials.NewTLS(p.MTLSClientConfig())
}

// Close releases the underlying source. For a Workload API source this closes
// the connection to the agent and stops the rotation stream.
func (p *Provider) Close() error {
	return p.source.Close()
}
