// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"testing"

	stdGrpc "google.golang.org/grpc"
	grpcmetadata "google.golang.org/grpc/metadata"
)

func TestServerInterceptor_Dependencies(t *testing.T) {
	si := ServerInterceptor()
	ic := si.(*interceptor)
	deps := ic.Dependencies()
	want := []string{"metadata", "errstatus"}
	if len(deps) != len(want) {
		t.Fatalf("Dependencies() = %v, want %v", deps, want)
	}
	for i, d := range deps {
		if d != want[i] {
			t.Fatalf("Dependencies()[%d] = %q, want %q", i, d, want[i])
		}
	}
}

func TestServerInterceptor_Name(t *testing.T) {
	si := ServerInterceptor()
	if si.Name() != "auth" {
		t.Fatalf("Name() = %q", si.Name())
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := captured.Headers["authorization"]; ok {
		t.Fatal("authorization header must not be present in Credentials.Headers")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, key := range []string{"x-request-id", "x-tenant-id"} {
		if _, ok := captured.Headers[key]; !ok {
			t.Errorf("expected header %q to be present in Credentials.Headers", key)
		}
	}

	if v := captured.Headers["x-request-id"]; v != "abc-123" {
		t.Errorf("x-request-id = %q, want %q", v, "abc-123")
	}
	if v := captured.Headers["x-tenant-id"]; v != "tenant-42" {
		t.Errorf("x-tenant-id = %q, want %q", v, "tenant-42")
	}
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
		if got := isSensitiveHeader(tt.key); got != tt.want {
			t.Errorf("isSensitiveHeader(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}
