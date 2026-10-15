// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	stdGrpc "google.golang.org/grpc"
	grpcmetadata "google.golang.org/grpc/metadata"
)

func TestServerInterceptor_Dependencies(t *testing.T) {
	t.Parallel()
	si := ServerInterceptor()
	ic := si.(*interceptor)
	deps := ic.Dependencies()
	want := []string{"metadata", "errstatus"}
	require.Equal(t, len(want), len(deps))
	for i, d := range deps {
		require.Equal(t, want[i], d)
	}
}

func TestServerInterceptor_Name(t *testing.T) {
	t.Parallel()
	si := ServerInterceptor()
	require.Equal(t, "auth", si.Name())
}

func TestAuthFunc_Interface(t *testing.T) {
	t.Parallel()
	var _ Auth = AuthFunc(nil)
}

func TestClientAuthFunc_Interface(t *testing.T) {
	t.Parallel()
	var _ ClientAuth = ClientAuthFunc(nil)
}

func TestTokenExtractorFunc_Interface(t *testing.T) {
	t.Parallel()
	var _ TokenExtractor = TokenExtractorFunc(nil)
}

func TestSensitiveHeadersExcluded(t *testing.T) {
	t.Parallel()
	var captured Credentials

	si := ServerInterceptor(
		WithTokenExtractor(TokenExtractorFunc(func(_ context.Context) (string, error) {
			return "test-token", nil
		})),
		WithAuthFn(AuthFunc(func(_ context.Context, _ Request) (any, error) {
			return "user-data", nil
		})),
		WithClientAuth(ClientAuthFunc(func(ctx context.Context, cred Credentials) (context.Context, error) {
			captured = cred
			return ctx, nil
		})),
	)

	md := grpcmetadata.Pairs(
		"authorization", "Bearer test-token",
		"x-request-id", "abc-123",
	)
	ctx := grpcmetadata.NewIncomingContext(t.Context(), md)

	unary := si.ServerUnaryInterceptor()
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }
	info := &stdGrpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	_, err := unary(ctx, nil, info, handler)
	require.NoError(t, err)

	_, ok := captured.Headers["authorization"]
	require.False(t, ok, "authorization header must not be present in Credentials.Headers")
}

func TestNonSensitiveHeadersPreserved(t *testing.T) {
	t.Parallel()
	var captured Credentials

	si := ServerInterceptor(
		WithTokenExtractor(TokenExtractorFunc(func(_ context.Context) (string, error) {
			return "test-token", nil
		})),
		WithAuthFn(AuthFunc(func(_ context.Context, _ Request) (any, error) {
			return "user-data", nil
		})),
		WithClientAuth(ClientAuthFunc(func(ctx context.Context, cred Credentials) (context.Context, error) {
			captured = cred
			return ctx, nil
		})),
	)

	md := grpcmetadata.Pairs(
		"authorization", "Bearer test-token",
		"x-request-id", "abc-123",
		"x-tenant-id", "tenant-42",
	)
	ctx := grpcmetadata.NewIncomingContext(t.Context(), md)

	unary := si.ServerUnaryInterceptor()
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }
	info := &stdGrpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}

	_, err := unary(ctx, nil, info, handler)
	require.NoError(t, err)

	for _, key := range []string{"x-request-id", "x-tenant-id"} {
		_, ok := captured.Headers[key]
		require.True(t, ok, "expected header %q to be present in Credentials.Headers", key)
	}

	v := captured.Headers["x-request-id"]
	require.Equal(t, "abc-123", v)
	v = captured.Headers["x-tenant-id"]
	require.Equal(t, "tenant-42", v)
}

func TestIsSensitiveHeader(t *testing.T) {
	t.Parallel()
	tests := []struct {
		key  string
		want bool
	}{
		{"authorization", true},
		{"Authorization", true},
		{"AUTHORIZATION", true},
		{"aUtHoRiZaTiOn", true},
		{"x-request-id", false},
		{"content-type", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isSensitiveHeader(tt.key)
		require.Equal(t, tt.want, got)
	}
}

// TestServerInterceptor_NoDefaultExemptions is a regression for SA-001: the
// interceptor used to default its ignore patterns to the shared reflection
// and health list, whose health pattern is unanchored at the end and also
// matched any service named /grpc.health.v1.Health<anything>/. Nothing is
// exempt unless configured.
func TestServerInterceptor_NoDefaultExemptions(t *testing.T) {
	t.Parallel()
	si := ServerInterceptor(
		WithAuthFn(AuthFunc(func(context.Context, Request) (any, error) {
			return nil, status.Error(codes.PermissionDenied, "denied")
		})),
	)
	unary := si.ServerUnaryInterceptor()
	stream := si.ServerStreamInterceptor()

	for _, method := range []string{
		"/grpc.health.v1.Health/Check",
		"/grpc.health.v1.Health/Watch",
		"/grpc.health.v1.HealthAdmin/Shutdown",
		"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
		"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo",
	} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			// Without a token the call is rejected before the auth function.
			_, err := unary(t.Context(), nil, &stdGrpc.UnaryServerInfo{FullMethod: method},
				func(context.Context, any) (any, error) {
					t.Error("unary handler must not run without authentication")
					return nil, nil
				})
			require.Equal(t, codes.Unauthenticated, status.Code(err))

			// With a token the configured auth function decides.
			ctx := grpcmetadata.NewIncomingContext(t.Context(), grpcmetadata.Pairs("authorization", "Bearer t"))
			_, err = unary(ctx, nil, &stdGrpc.UnaryServerInfo{FullMethod: method},
				func(context.Context, any) (any, error) {
					t.Error("unary handler must not run when authentication fails")
					return nil, nil
				})
			require.Equal(t, codes.PermissionDenied, status.Code(err))

			err = stream(nil, &ctxStream{ctx: ctx}, &stdGrpc.StreamServerInfo{FullMethod: method, IsClientStream: true},
				func(any, stdGrpc.ServerStream) error {
					t.Error("stream handler must not run when authentication fails")
					return nil
				})
			require.Equal(t, codes.PermissionDenied, status.Code(err))
		})
	}
}

// ctxStream is a minimal [stdGrpc.ServerStream] carrying only a context.
type ctxStream struct {
	stdGrpc.ServerStream
	ctx context.Context
}

func (s *ctxStream) Context() context.Context { return s.ctx }

// TestServerInterceptor_LegacyIgnorePatternsOptIn pins the explicit opt-in
// that restores the pre-SA-001 exemptions.
func TestServerInterceptor_LegacyIgnorePatternsOptIn(t *testing.T) {
	t.Parallel()
	unary := ServerInterceptor(WithIgnorePatterns(defaults.IgnorePatterns...)).ServerUnaryInterceptor()
	handler := func(context.Context, any) (any, error) { return "ok", nil }

	resp, err := unary(t.Context(), nil, &stdGrpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}, handler)
	require.NoError(t, err)
	require.Equal(t, "ok", resp)

	_, err = unary(t.Context(), nil, &stdGrpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}, handler)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}
