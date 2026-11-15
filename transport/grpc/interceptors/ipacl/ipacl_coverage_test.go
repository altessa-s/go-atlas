// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ipacl_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/netip"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/clientip"
	"github.com/altessa-s/go-atlas/transport/policy/ipacl"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grpcipacl "github.com/altessa-s/go-atlas/transport/grpc/interceptors/ipacl"
)

type ctxStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *ctxStream) Context() context.Context { return s.ctx }

func registry() *ipacl.Registry {
	reg := ipacl.NewRegistry(ipacl.PolicyDeny)
	reg.Register("/svc.A/Get", &ipacl.AccessRule{Allowlist: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}})
	return reg
}

func TestName(t *testing.T) {
	t.Parallel()
	require.Equal(t, "ipacl", grpcipacl.Name())
	require.Equal(t, "ipacl", grpcipacl.ID.Name())
}

func TestStreamInterceptor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ip       string
		method   string
		opts     []grpcipacl.Option
		wantCode codes.Code
	}{
		{name: "allowed", ip: "10.2.3.4", method: "/svc.A/Get"},
		{name: "denied by allowlist", ip: "192.0.2.1", method: "/svc.A/Get", wantCode: codes.PermissionDenied},
		{name: "unregistered method denied by policy", ip: "10.2.3.4", method: "/svc.A/Other", wantCode: codes.PermissionDenied},
		{name: "no client IP denied", method: "/svc.A/Get", wantCode: codes.PermissionDenied},
		{
			name: "ignored by pattern", ip: "192.0.2.1", method: "/svc.A/Get",
			opts: []grpcipacl.Option{grpcipacl.WithIgnorePatterns(regexp.MustCompile(`^/svc\.a/`))},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			if tc.ip != "" {
				ctx = clientip.NewContext(ctx, netip.MustParseAddr(tc.ip))
			}
			called := false
			err := grpcipacl.ServerStreamInterceptor(registry(), tc.opts...)(nil, &ctxStream{ctx: ctx},
				&grpc.StreamServerInfo{FullMethod: tc.method},
				func(any, grpc.ServerStream) error { called = true; return nil })
			require.Equal(t, tc.wantCode, status.Code(err))
			require.Equal(t, tc.wantCode == codes.OK, called)
		})
	}
}

func TestUnaryInterceptor_PackageHelperAndLogger(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	unary := grpcipacl.ServerUnaryInterceptor(registry(), grpcipacl.WithLogger(logger))

	ctx := clientip.NewContext(t.Context(), netip.MustParseAddr("192.0.2.9"))
	_, err := unary(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/svc.A/Get"},
		func(context.Context, any) (any, error) { return "ok", nil })
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.Contains(t, buf.String(), "access denied")

	ctx = clientip.NewContext(t.Context(), netip.MustParseAddr("10.0.0.1"))
	resp, err := unary(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/svc.A/Get"},
		func(context.Context, any) (any, error) { return "ok", nil })
	require.NoError(t, err)
	require.Equal(t, "ok", resp)
	require.Contains(t, buf.String(), "access granted")
}
