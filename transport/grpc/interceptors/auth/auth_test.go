// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	stdGrpc "google.golang.org/grpc"
	grpcmetadata "google.golang.org/grpc/metadata"
)

func TestServerInterceptor_Dependencies(t *testing.T) {
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
	si := ServerInterceptor()
	require.Equal(t, "auth", si.Name())
}

func TestAuthFunc_Interface(t *testing.T) {
	var _ Auth = AuthFunc(nil)
}

func TestClientAuthFunc_Interface(t *testing.T) {
	var _ ClientAuth = ClientAuthFunc(nil)
}

func TestTokenExtractorFunc_Interface(t *testing.T) {
	var _ TokenExtractor = TokenExtractorFunc(nil)
}

func TestSensitiveHeadersExcluded(t *testing.T) {
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
