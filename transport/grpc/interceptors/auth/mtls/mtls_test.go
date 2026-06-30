// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/audit"
	"github.com/altessa-s/go-atlas/auth/spiffe"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/mtls"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
	authgrpc "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
)

func ctxWithVerifiedCert(ctx context.Context, cert *x509.Certificate) context.Context {
	return peer.NewContext(ctx, &peer.Peer{
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{cert}}},
		},
	})
}

func spiffeCert(t *testing.T, id string) *x509.Certificate {
	t.Helper()
	u, err := url.Parse(id)
	require.NoError(t, err)
	return &x509.Certificate{URIs: []*url.URL{u}}
}

func TestAuthFuncSPIFFEDefault(t *testing.T) {
	t.Parallel()
	ctx := ctxWithVerifiedCert(t.Context(), spiffeCert(t, "spiffe://example.org/ns/default/sa/billing"))

	principal, err := mtls.AuthFunc()(ctx, authgrpc.Request{})
	require.NoError(t, err)

	id, ok := principal.(spiffe.ID)
	require.True(t, ok)
	require.Equal(t, "spiffe://example.org/ns/default/sa/billing", id.String())
}

func TestAuthFuncWithIdentity(t *testing.T) {
	t.Parallel()
	ctx := ctxWithVerifiedCert(t.Context(), spiffeCert(t, "spiffe://example.org/ns/default/sa/billing"))

	authFn := mtls.AuthFunc(coremtls.WithIdentity(func(c *x509.Certificate) (any, error) {
		id, err := spiffe.IDFromCertificate(c)
		return id.Path, err
	}))
	principal, err := authFn(ctx, authgrpc.Request{})
	require.NoError(t, err)
	require.Equal(t, "/ns/default/sa/billing", principal)
}

func TestAuthFuncUnauthenticated(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ctx  func(t *testing.T) context.Context
	}{
		{"no peer", func(t *testing.T) context.Context { return t.Context() }},
		{"not tls", func(t *testing.T) context.Context {
			return peer.NewContext(t.Context(), &peer.Peer{})
		}},
		{"no verified cert", func(t *testing.T) context.Context {
			return peer.NewContext(t.Context(), &peer.Peer{AuthInfo: credentials.TLSInfo{}})
		}},
		{"cert without spiffe san", func(t *testing.T) context.Context {
			return ctxWithVerifiedCert(t.Context(), &x509.Certificate{})
		}},
		{"untrusted trust domain", func(t *testing.T) context.Context {
			return ctxWithVerifiedCert(t.Context(), spiffeCert(t, "spiffe://evil.example/x"))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			authFn := mtls.AuthFunc(coremtls.WithValidator(coremtls.TrustDomainValidator("example.org")))
			_, err := authFn(tc.ctx(t), authgrpc.Request{})
			require.Equal(t, codes.Unauthenticated, status.Code(err))
		})
	}
}

func TestAuthFuncRequiredAuditMapsToInternal(t *testing.T) {
	t.Parallel()
	rec := audit.NewRecorder(
		audit.SinkFunc(func(context.Context, audit.Decision) error { return errors.New("sink down") }),
		audit.WithPolicyMode(audit.PolicyAll), audit.WithFailureMode(audit.FailureRequired),
	)
	authFn := mtls.AuthFunc(coremtls.WithAudit(rec, nil))
	ctx := ctxWithVerifiedCert(t.Context(), spiffeCert(t, "spiffe://example.org/x"))
	_, err := authFn(ctx, authgrpc.Request{})
	require.Equal(t, codes.Internal, status.Code(err))
}

func TestPeerCertificate(t *testing.T) {
	t.Parallel()
	cert := spiffeCert(t, "spiffe://example.org/x")
	got, err := mtls.PeerCertificate(ctxWithVerifiedCert(t.Context(), cert))
	require.NoError(t, err)
	require.Same(t, cert, got)

	_, err = mtls.PeerCertificate(t.Context())
	require.ErrorIs(t, err, mtls.ErrNoPeer)
}
