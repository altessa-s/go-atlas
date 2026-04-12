// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package geoacl

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"
	"github.com/altessa-s/go-atlas/transport/internal/geoacl"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockResolver struct {
	geo geoacl.GeoInfo
	err error
}

func (m *mockResolver) Resolve(_ context.Context, _ netip.Addr) (geoacl.GeoInfo, error) {
	return m.geo, m.err
}

func newTestRegistry() *geoacl.Registry {
	reg := geoacl.NewRegistry(geoacl.PolicyDeny)
	reg.Register("/test.Service/Allowed", &geoacl.AccessRule{
		AllowCountries: []string{"US"},
	})
	reg.Register("/test.Service/Denied", &geoacl.AccessRule{
		DenyCountries: []string{"US"},
	})
	return reg
}

func newTestResolver() *mockResolver {
	return &mockResolver{geo: geoacl.GeoInfo{ContinentCode: "NA", CountryCode: "US", RegionCode: "CA"}}
}

func noopHandler(_ context.Context, _ any) (any, error) {
	return "ok", nil
}

func TestUnaryInterceptor_Allowed(t *testing.T) {
	inter := ServerInterceptor(newTestResolver(), newTestRegistry())
	ctx := clientip.NewContext(t.Context(), netip.MustParseAddr("10.1.1.1"))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Allowed"}

	resp, err := inter.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
	require.NoError(t, err)
	require.Equal(t, "ok", resp)
}

func TestUnaryInterceptor_Denied(t *testing.T) {
	inter := ServerInterceptor(newTestResolver(), newTestRegistry())
	ctx := clientip.NewContext(t.Context(), netip.MustParseAddr("10.1.1.1"))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Denied"}

	_, err := inter.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
	require.NotNil(t, err, "expected error")
	s, ok := status.FromError(err)
	require.True(t, ok, "code = %v, want PermissionDenied", s.Code())
	require.Equal(t, codes.PermissionDenied, s.Code())
}

func TestUnaryInterceptor_NoIP_FallbackDeny(t *testing.T) {
	inter := ServerInterceptor(newTestResolver(), newTestRegistry(), WithFallbackBehavior(fallback.Deny))
	ctx := t.Context() // no IP in context
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Allowed"}

	_, err := inter.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
	require.NotNil(t, err, "expected error when no IP and fallback deny")
	s, ok := status.FromError(err)
	require.True(t, ok, "code = %v, want PermissionDenied", s.Code())
	require.Equal(t, codes.PermissionDenied, s.Code())
}

func TestUnaryInterceptor_NoIP_FallbackAllow(t *testing.T) {
	inter := ServerInterceptor(newTestResolver(), newTestRegistry(), WithFallbackBehavior(fallback.Allow))
	ctx := t.Context() // no IP in context
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Allowed"}

	resp, err := inter.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
	require.NoError(t, err)
	require.Equal(t, "ok", resp)
}

func TestUnaryInterceptor_ResolverError_FallbackDeny(t *testing.T) {
	resolver := &mockResolver{err: errors.New("geo lookup failed")}
	inter := ServerInterceptor(resolver, newTestRegistry(), WithFallbackBehavior(fallback.Deny))
	ctx := clientip.NewContext(t.Context(), netip.MustParseAddr("10.1.1.1"))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Allowed"}

	_, err := inter.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
	require.NotNil(t, err, "expected error on resolver failure with fallback deny")
	s, ok := status.FromError(err)
	require.True(t, ok, "code = %v, want PermissionDenied", s.Code())
	require.Equal(t, codes.PermissionDenied, s.Code())
}

func TestUnaryInterceptor_ResolverError_FallbackAllow(t *testing.T) {
	resolver := &mockResolver{err: errors.New("geo lookup failed")}
	inter := ServerInterceptor(resolver, newTestRegistry(), WithFallbackBehavior(fallback.Allow))
	ctx := clientip.NewContext(t.Context(), netip.MustParseAddr("10.1.1.1"))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Allowed"}

	resp, err := inter.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
	require.NoError(t, err)
	require.Equal(t, "ok", resp)
}

func TestUnaryInterceptor_ResolverError_FallbackError(t *testing.T) {
	resolver := &mockResolver{err: errors.New("geo lookup failed")}
	inter := ServerInterceptor(resolver, newTestRegistry(), WithFallbackBehavior(fallback.Error))
	ctx := clientip.NewContext(t.Context(), netip.MustParseAddr("10.1.1.1"))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Allowed"}

	_, err := inter.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
	require.NotNil(t, err, "expected error on resolver failure with fallback error")
	s, ok := status.FromError(err)
	require.True(t, ok, "code = %v, want Internal", s.Code())
	require.Equal(t, codes.Internal, s.Code())
	require.True(t, strings.Contains(s.Message(), "geo lookup failed"), "message = %q, want it to contain resolver error", s.Message())
}

func TestUnaryInterceptor_IgnoredMethod(t *testing.T) {
	inter := ServerInterceptor(newTestResolver(), newTestRegistry(), WithIgnoreMethods("/test.Service/Denied"))
	ctx := clientip.NewContext(t.Context(), netip.MustParseAddr("1.2.3.4"))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Denied"}

	resp, err := inter.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
	require.NoError(t, err)
	require.Equal(t, "ok", resp)
}

func TestStreamInterceptor_NotNil(t *testing.T) {
	inter := ServerInterceptor(newTestResolver(), newTestRegistry())
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
