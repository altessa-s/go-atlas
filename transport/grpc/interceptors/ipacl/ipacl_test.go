// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ipacl

import (
	"context"
	"net/netip"
	"testing"

	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"
	"github.com/altessa-s/go-atlas/transport/internal/ipacl"

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
	ctx := clientip.NewContext(context.Background(), netip.MustParseAddr("10.1.1.1"))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Allowed"}

	resp, err := inter.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("resp = %v, want ok", resp)
	}
}

func TestUnaryInterceptor_Denied(t *testing.T) {
	inter := ServerInterceptor(newTestRegistry())
	ctx := clientip.NewContext(context.Background(), netip.MustParseAddr("192.168.1.1"))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Allowed"}

	_, err := inter.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
	if err == nil {
		t.Fatal("expected error")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", s.Code())
	}
}

func TestUnaryInterceptor_NoIP_FallbackDeny(t *testing.T) {
	inter := ServerInterceptor(newTestRegistry(), WithFallbackBehavior(fallback.Deny))
	ctx := context.Background() // no IP in context
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Allowed"}

	_, err := inter.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
	if err == nil {
		t.Fatal("expected error when no IP and fallback deny")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", s.Code())
	}
}

func TestUnaryInterceptor_NoIP_FallbackAllow(t *testing.T) {
	inter := ServerInterceptor(newTestRegistry(), WithFallbackBehavior(fallback.Allow))
	ctx := context.Background() // no IP in context
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Allowed"}

	resp, err := inter.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("resp = %v, want ok", resp)
	}
}

func TestUnaryInterceptor_IgnoredMethod(t *testing.T) {
	inter := ServerInterceptor(newTestRegistry(), WithIgnoreMethods("/test.Service/Denied"))
	ctx := clientip.NewContext(context.Background(), netip.MustParseAddr("1.2.3.4"))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Denied"}

	resp, err := inter.ServerUnaryInterceptor()(ctx, nil, info, noopHandler)
	if err != nil {
		t.Fatalf("unexpected error for ignored method: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("resp = %v, want ok", resp)
	}
}

func TestStreamInterceptor_NotNil(t *testing.T) {
	inter := ServerInterceptor(newTestRegistry())
	if inter.ServerStreamInterceptor() == nil {
		t.Fatal("ServerStreamInterceptor should not be nil")
	}
}

func TestInterceptor_Dependencies(t *testing.T) {
	i := &interceptor{}
	deps := i.Dependencies()
	if len(deps) != 1 || deps[0] != "realip" {
		t.Fatalf("Dependencies() = %v, want [realip]", deps)
	}
}
