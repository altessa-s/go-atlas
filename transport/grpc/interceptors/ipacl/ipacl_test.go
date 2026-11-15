// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ipacl

import (
	"context"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/clientip"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"
	"github.com/altessa-s/go-atlas/transport/policy/fallback"
	"github.com/altessa-s/go-atlas/transport/policy/ipacl"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestRegistry() *ipacl.Registry {
	reg := ipacl.NewRegistry(ipacl.PolicyDeny)
	reg.Register("/test.Service/Allowed", &ipacl.AccessRule{
		Allowlist: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
	})
	reg.Register("/test.Service/Denied", &ipacl.AccessRule{
		Denylist: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")},
	})
	return reg
}

func noopHandler(_ context.Context, _ any) (any, error) {
	return "ok", nil
}

func TestUnaryInterceptor_Allowed(t *testing.T) {
	inter := ServerInterceptor(newTestRegistry())
	ctx := clientip.NewContext(t.Context(), netip.MustParseAddr("10.1.1.1"))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Allowed"}

	resp, err := inter.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
	require.NoError(t, err)
	require.Equal(t, "ok", resp)
}

func TestUnaryInterceptor_Denied(t *testing.T) {
	inter := ServerInterceptor(newTestRegistry())
	ctx := clientip.NewContext(t.Context(), netip.MustParseAddr("192.168.1.1"))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Allowed"}

	_, err := inter.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
	require.NotNil(t, err, "expected error")
	s, ok := status.FromError(err)
	require.True(t, ok, "code = %v, want PermissionDenied", s.Code())
	require.Equal(t, codes.PermissionDenied, s.Code())
}

func TestUnaryInterceptor_NoIP_FallbackDeny(t *testing.T) {
	inter := ServerInterceptor(newTestRegistry(), WithFallbackBehavior(fallback.Deny))
	ctx := t.Context() // no IP in context
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Allowed"}

	_, err := inter.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
	require.NotNil(t, err, "expected error when no IP and fallback deny")
	s, ok := status.FromError(err)
	require.True(t, ok, "code = %v, want PermissionDenied", s.Code())
	require.Equal(t, codes.PermissionDenied, s.Code())
}

func TestUnaryInterceptor_NoIP_FallbackAllow(t *testing.T) {
	inter := ServerInterceptor(newTestRegistry(), WithFallbackBehavior(fallback.Allow))
	ctx := t.Context() // no IP in context
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Allowed"}

	resp, err := inter.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
	require.NoError(t, err)
	require.Equal(t, "ok", resp)
}

func TestUnaryInterceptor_IgnoredMethod(t *testing.T) {
	inter := ServerInterceptor(newTestRegistry(), WithIgnoreMethods("/test.Service/Denied"))
	ctx := clientip.NewContext(t.Context(), netip.MustParseAddr("1.2.3.4"))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Denied"}

	resp, err := inter.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
	require.NoError(t, err)
	require.Equal(t, "ok", resp)
}

func TestStreamInterceptor_NotNil(t *testing.T) {
	inter := ServerInterceptor(newTestRegistry())
	require.NotNil(t, inter.ServerStreamInterceptor(), "ServerStreamInterceptor should not be nil")
}

func TestInterceptor_Dependencies(t *testing.T) {
	i := &interceptor{}
	deps := i.Dependencies()
	require.Len(t, deps, 0)
	reqDeps := i.RequiredDependencies()
	require.Len(t, reqDeps, 1)
	require.Equal(t, "realip", reqDeps[0])
}

// TestUnaryInterceptor_NoDefaultExemptions pins that reflection and health
// are guarded like any other method unless the caller opts in to
// defaults.IgnorePatterns.
func TestUnaryInterceptor_NoDefaultExemptions(t *testing.T) {
	t.Parallel()

	for _, method := range []string{
		"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
		"/grpc.health.v1.Health/Check",
	} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			ctx := clientip.NewContext(t.Context(), netip.MustParseAddr("1.2.3.4"))
			info := &grpc.UnaryServerInfo{FullMethod: method}

			_, err := ServerInterceptor(newTestRegistry()).ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
			require.Equal(t, codes.PermissionDenied, status.Code(err))

			optIn := ServerInterceptor(newTestRegistry(), WithIgnorePatterns(defaults.IgnorePatterns...))
			resp, err := optIn.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
			require.NoError(t, err)
			require.Equal(t, "ok", resp)
		})
	}
}
