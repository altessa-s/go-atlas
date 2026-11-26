// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/transport/grpc/client"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"

	"golang.org/x/oauth2"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	grpchealth "google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// TestTokenOverPlaintextIsRefused configures a token on a plaintext client:
// the call must fail and the server must never see the token.
func TestTokenOverPlaintextIsRefused(t *testing.T) {
	t.Parallel()

	b := startBackend(t)
	c, err := client.New(t.Context(), "passthrough:///bufnet",
		client.WithInsecure(), client.WithoutHealthCheck(),
		client.WithDialOptions(b.dialer(), grpc.WithPerRPCCredentials(tokenCredentials(t, auth.StaticTokenProvider("secret")))),
	)
	if err == nil {
		t.Cleanup(func() { _ = c.Close(context.WithoutCancel(t.Context())) })
		_, err = invoke(t, c, "hi")
	}
	require.Error(t, err)
	require.Empty(t, b.lastAuthorization(), "the token reached a plaintext server")
}

// TestInsecureTokenCredentialsAreTheExplicitOptIn pins the named way to send a
// token in cleartext.
func TestInsecureTokenCredentialsAreTheExplicitOptIn(t *testing.T) {
	t.Parallel()

	b := startBackend(t)
	creds := client.NewInsecureTokenCredentials(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "dev-token"}))
	c := newClient(t, b, client.WithDialOptions(grpc.WithPerRPCCredentials(creds)))

	_, err := invoke(t, c, "hi")
	require.NoError(t, err)
	require.Equal(t, "Bearer dev-token", b.lastAuthorization())
}

func tokenCredentials(t *testing.T, p auth.TokenProvider) credentials.PerRPCCredentials {
	t.Helper()
	creds, err := auth.NewPerRPCCredentials(p)
	require.NoError(t, err)
	return creds
}

// authServer is a TLS health server that records the authorization values of
// every call it receives.
type authServer struct {
	addr string
	ca   *testhelpers.CA

	mu   sync.Mutex
	seen [][]string
}

func startAuthServer(t *testing.T) *authServer {
	t.Helper()
	ca := testhelpers.NewCA(t)
	cert := ca.SignLeaf(t, testhelpers.WithDNSNames("localhost"))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	s := &authServer{addr: lis.Addr().String(), ca: ca}
	srv := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}})),
		grpc.UnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
			md, _ := metadata.FromIncomingContext(ctx)
			s.mu.Lock()
			s.seen = append(s.seen, md.Get("authorization"))
			s.mu.Unlock()
			return h(ctx, req)
		}),
	)
	healthpb.RegisterHealthServer(srv, grpchealth.NewServer())
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return s
}

func (s *authServer) calls() [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.seen)
}

func (s *authServer) check(t *testing.T, ctx context.Context, p auth.TokenProvider) error {
	t.Helper()
	c, err := client.New(t.Context(), s.addr, client.WithoutHealthCheck(),
		client.WithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12, RootCAs: s.ca.Pool, ServerName: "localhost"}),
		client.WithDialOptions(grpc.WithPerRPCCredentials(tokenCredentials(t, p))),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close(context.WithoutCancel(t.Context())) })
	conn, err := c.GetConnection(t.Context())
	require.NoError(t, err)
	defer c.ReturnConnection(conn)
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err = healthpb.NewHealthClient(conn).Check(callCtx, &healthpb.HealthCheckRequest{})
	return err
}

func TestTokenCredentialsOverTLS(t *testing.T) {
	t.Parallel()
	srv := startAuthServer(t)
	require.NoError(t, srv.check(t, t.Context(), auth.StaticTokenProvider("tls-token")))
	require.Equal(t, [][]string{{"Bearer tls-token"}}, srv.calls())
}

// TestTokenCredentialsRejectConflictingHeader sends a call that already carries
// an authorization value: it must fail before transmission, never send two.
func TestTokenCredentialsRejectConflictingHeader(t *testing.T) {
	t.Parallel()
	srv := startAuthServer(t)
	ctx := metadata.AppendToOutgoingContext(t.Context(), "authorization", "Bearer caller")
	err := srv.check(t, ctx, auth.StaticTokenProvider("tls-token"))
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	require.Empty(t, srv.calls())
}

// TestTokenCredentialsProviderErrorStaysOutOfStatus fails the provider with a
// marker that must not reach the RPC status.
func TestTokenCredentialsProviderErrorStaysOutOfStatus(t *testing.T) {
	t.Parallel()
	const marker = "SECRET-MARKER-19c4"
	srv := startAuthServer(t)
	err := srv.check(t, t.Context(), auth.TokenProviderFunc(func(context.Context) (string, error) {
		return "", errors.New("vault: " + marker)
	}))
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	require.NotContains(t, err.Error(), marker)
	require.Empty(t, srv.calls())
}
