// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/altessa-s/go-atlas/audit/decision"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
)

var (
	// ErrNoPeer indicates the context carries no gRPC peer information.
	ErrNoPeer = errors.New("mtls: no peer in context")
	// ErrNoTLS indicates the connection is not (mutual) TLS.
	ErrNoTLS = errors.New("mtls: connection is not mutual TLS")
	// ErrNoVerifiedCert indicates the peer presented no verified client
	// certificate — the client was not required to authenticate, or verification
	// did not run.
	ErrNoVerifiedCert = errors.New("mtls: no verified client certificate")
)

// PeerCertificate returns the verified leaf client certificate from ctx. It
// reads the verified chain, which the TLS stack populates only when the server
// is configured to require and verify the client certificate
// (tls.RequireAndVerifyClientCert) — never the unverified PeerCertificates. A
// non-mTLS or unauthenticated peer yields an error.
func PeerCertificate(ctx context.Context) (*x509.Certificate, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, ErrNoPeer
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return nil, ErrNoTLS
	}
	chains := tlsInfo.State.VerifiedChains
	if len(chains) == 0 || len(chains[0]) == 0 {
		return nil, ErrNoVerifiedCert
	}
	return chains[0][0], nil
}

// AuthFunc returns an [auth.AuthFunc] that authenticates the caller from its
// verified mTLS client certificate, delegating identity derivation, validation,
// and audit to a [github.com/altessa-s/go-atlas/auth/mtls.Authenticator] built
// from opts. Configure it with that package's options — [coremtls.WithIdentity],
// [coremtls.WithValidator], [coremtls.WithAudit] — the transport label is set to
// "grpc" automatically. opts must make the trust decision
// ([coremtls.WithTrustDomains] or [coremtls.WithInsecureAnyTrustDomain]);
// validators do not count as one, and without it AuthFunc returns an error
// wrapping [coremtls.ErrNoTrustDomainPolicy] instead of an auth function.
//
// A missing peer, non-mTLS connection, or no verified certificate, and any
// identity or validator failure, map to codes.Unauthenticated (fail-closed). A
// required-audit failure on an otherwise-successful authentication maps to
// codes.Internal so nothing proceeds unrecorded.
//
// Identity is read from the connection's verified certificate chain, not from
// request metadata, so the [auth.Request] argument is unused.
func AuthFunc(opts ...coremtls.Option) (auth.AuthFunc, error) {
	authenticator, err := coremtls.NewAuthenticator(append([]coremtls.Option{coremtls.WithTransport("grpc")}, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("mtls: build authenticator: %w", err)
	}
	return func(ctx context.Context, _ auth.Request) (any, error) {
		cert, err := PeerCertificate(ctx)
		if err != nil {
			// Never surface the underlying TLS/identity error text to the caller:
			// it can reveal server TLS configuration or certificate chain details.
			return nil, status.Error(codes.Unauthenticated, "unauthenticated")
		}
		principal, err := authenticator.Authenticate(ctx, cert)
		if err != nil {
			if errors.Is(err, decision.ErrAuditFailed) {
				return nil, status.Error(codes.Internal, "mtls: audit failed")
			}
			return nil, status.Error(codes.Unauthenticated, "unauthenticated")
		}
		return principal, nil
	}, nil
}

// PeerCredential is the token extractor of a server that authenticates calls by their client certificate alone. The
// auth interceptor wants a credential before it runs the auth function; PeerCredential supplies the serial number of
// the verified peer certificate, which [AuthFunc] then reads from the connection itself. A call without a verified
// client certificate fails with codes.Unauthenticated before the auth function runs.
//
//	grpcauth.ServerInterceptor(grpcauth.WithTokenExtractor(mtls.PeerCredential()), grpcauth.WithAuthFn(authFn))
func PeerCredential() auth.TokenExtractor {
	return auth.TokenExtractorFunc(func(ctx context.Context) (string, error) {
		cert, err := PeerCertificate(ctx)
		if err != nil {
			return "", status.Error(codes.Unauthenticated, "unauthenticated")
		}
		return cert.SerialNumber.String(), nil
	})
}
