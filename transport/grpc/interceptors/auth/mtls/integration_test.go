// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls_test

import (
	"context"
	"crypto/tls"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/spiffe"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/mtls"
	"github.com/altessa-s/go-atlas/transport/internal/mtlstest"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	coremtls "github.com/altessa-s/go-atlas/auth/mtls"
	authgrpc "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
)

// principalKey carries the authenticated principal from the interceptor to the
// handler within the test.
type principalKey struct{}

// whoamiHandler is the hand-rolled service method: it echoes the principal that
// the auth interceptor installed in the context, so the client can assert the
// peer certificate produced the expected identity end to end.
func whoamiHandler(_ any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	if err := dec(new(emptypb.Empty)); err != nil {
		return nil, err
	}
	h := func(ctx context.Context, _ any) (any, error) {
		id, _ := ctx.Value(principalKey{}).(spiffe.ID)
		return wrapperspb.String(id.String()), nil
	}
	if interceptor == nil {
		return h(ctx, nil)
	}
	return interceptor(ctx, new(emptypb.Empty), &grpc.UnaryServerInfo{FullMethod: "/e2e.Echo/Whoami"}, h)
}

var echoDesc = grpc.ServiceDesc{
	ServiceName: "e2e.Echo",
	HandlerType: (*any)(nil),
	Methods:     []grpc.MethodDesc{{MethodName: "Whoami", Handler: whoamiHandler}},
}

// TestAuthFuncRealHandshake drives AuthFunc over a genuine mutual-TLS gRPC
// handshake: a real server requires and verifies the client certificate, so the
// peer's VerifiedChains are populated by the gRPC TLS transport itself (not
// synthesized). The interceptor derives the principal from that verified
// certificate and the handler echoes it back, asserting the SPIFFE identity in
// the client certificate's URI SAN reaches application code.
func TestAuthFuncRealHandshake(t *testing.T) {
	t.Parallel()
	ca := mtlstest.NewCA(t)
	serverCert := ca.ServerCert(t, net.IPv4(127, 0, 0, 1))
	spiffeID := "spiffe://example.org/ns/default/sa/billing"
	clientCert := ca.ClientCert(t, spiffeID)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	authFn := mtls.AuthFunc(coremtls.WithValidator(coremtls.TrustDomainValidator("example.org")))
	authInterceptor := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		principal, err := authFn(ctx, authgrpc.Request{})
		if err != nil {
			return nil, err
		}
		return handler(context.WithValue(ctx, principalKey{}, principal), req)
	}

	srv := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{serverCert},
			ClientCAs:    ca.Pool,
			ClientAuth:   tls.RequireAndVerifyClientCert,
			MinVersion:   tls.VersionTLS13,
		})),
		grpc.ChainUnaryInterceptor(authInterceptor),
	)
	srv.RegisterService(&echoDesc, nil)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(ln.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      ca.Pool,
		MinVersion:   tls.VersionTLS13,
	})))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	var reply wrapperspb.StringValue
	err = conn.Invoke(t.Context(), "/e2e.Echo/Whoami", &emptypb.Empty{}, &reply)
	require.NoError(t, err)
	require.Equal(t, spiffeID, reply.GetValue()) // peer cert → spiffe.ID principal, end to end
}
