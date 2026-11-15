// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/requestid"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	stdGrpc "google.golang.org/grpc"
)

// White-box access is needed to exercise the configured interceptor without starting a server.
func TestRequestIDInterceptorGenerateIfMissing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		generate bool
		want     codes.Code
	}{
		{name: "enabled generates", generate: true, want: codes.OK},
		{name: "disabled rejects a missing id", generate: false, want: codes.InvalidArgument},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := grpcconfig.DefaultRequestIDInterceptor()
			cfg.Enabled = true
			cfg.GenerateIfMissing = tc.generate
			b := New(&grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{RequestId: &cfg}}).WithRequestIDInterceptor()
			require.Len(t, b.interceptors, 1)

			unary := b.interceptors[0].ServerUnaryInterceptor()
			_, err := unary(t.Context(), nil, &stdGrpc.UnaryServerInfo{FullMethod: "/svc.v1.S/M"},
				func(context.Context, any) (any, error) { return nil, nil })
			require.Equal(t, tc.want, status.Code(err))
		})
	}
}

// The interceptor reads the request ID from the configured metadata key,
// x-request-id by default, and rejects one sent under any other key.
func TestRequestIDInterceptorHeaderName(t *testing.T) {
	t.Parallel()
	const id = "550e8400-e29b-41d4-a716-446655440000"
	for _, tc := range []struct {
		name       string
		headerName string // empty keeps the default
		sentKey    string
		want       codes.Code
	}{
		{name: "default key", sentKey: "x-request-id", want: codes.OK},
		{name: "default ignores another key", sentKey: "request-id", want: codes.InvalidArgument},
		{name: "configured key", headerName: "X-Correlation-ID", sentKey: "x-correlation-id", want: codes.OK},
		{name: "configured ignores the default key", headerName: "X-Correlation-ID", sentKey: "x-request-id", want: codes.InvalidArgument},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := grpcconfig.DefaultRequestIDInterceptor()
			cfg.Enabled = true
			cfg.GenerateIfMissing = false
			if tc.headerName != "" {
				cfg.HeaderName = tc.headerName
			}
			b := New(&grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{RequestId: &cfg}}).WithRequestIDInterceptor()
			require.Len(t, b.interceptors, 1)

			ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(tc.sentKey, id))
			var got string
			unary := b.interceptors[0].ServerUnaryInterceptor()
			_, err := unary(ctx, nil, &stdGrpc.UnaryServerInfo{FullMethod: "/svc.v1.S/M"},
				func(ctx context.Context, _ any) (any, error) {
					got = requestid.FromContext(ctx)
					return nil, nil
				})
			require.Equal(t, tc.want, status.Code(err))
			if tc.want == codes.OK {
				require.Equal(t, id, got)
			}
		})
	}
}
