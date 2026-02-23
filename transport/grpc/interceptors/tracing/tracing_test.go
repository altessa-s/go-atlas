// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestParseMethod(t *testing.T) {
	tests := []struct {
		name        string
		fullMethod  string
		wantService string
		wantMethod  string
	}{
		{"standard", "/mypackage.MyService/MyMethod", "mypackage.MyService", "MyMethod"},
		{"no_leading_slash", "mypackage.MyService/MyMethod", "", "mypackage.MyService/MyMethod"},
		{"empty", "", "", ""},
		{"only_slash", "/", "", ""},
		{"no_method_slash", "/ServiceOnly", "", "ServiceOnly"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, method := parseMethod(tt.fullMethod)
			if svc != tt.wantService {
				t.Fatalf("service = %q, want %q", svc, tt.wantService)
			}
			if method != tt.wantMethod {
				t.Fatalf("method = %q, want %q", method, tt.wantMethod)
			}
		})
	}
}

func TestSpanAttributes(t *testing.T) {
	attrs := spanAttributes("/pkg.Svc/Method")
	if len(attrs) != 3 {
		t.Fatalf("len = %d, want 3", len(attrs))
	}
}

func TestMetadataCarrier(t *testing.T) {
	md := metadata.New(nil)
	carrier := metadataCarrier(md)

	carrier.Set("traceparent", "00-abc-def-01")
	if got := carrier.Get("traceparent"); got != "00-abc-def-01" {
		t.Fatalf("Get = %q", got)
	}

	if got := carrier.Get("nonexistent"); got != "" {
		t.Fatalf("Get nonexistent = %q", got)
	}

	keys := carrier.Keys()
	if len(keys) != 1 {
		t.Fatalf("Keys len = %d, want 1", len(keys))
	}
}

func TestDefaultSpanName(t *testing.T) {
	if got := defaultSpanName("/pkg.Svc/Method"); got != "/pkg.Svc/Method" {
		t.Fatalf("defaultSpanName = %q", got)
	}
}

func TestConstants(t *testing.T) {
	if RPCSystemKey != "rpc.system" {
		t.Fatal("wrong RPCSystemKey")
	}
	if RPCSystemGRPC != "grpc" {
		t.Fatal("wrong RPCSystemGRPC")
	}
	if RPCServiceKey != "rpc.service" {
		t.Fatal("wrong RPCServiceKey")
	}
	if RPCMethodKey != "rpc.method" {
		t.Fatal("wrong RPCMethodKey")
	}
	if RPCGRPCStatusCodeKey != "rpc.grpc.status_code" {
		t.Fatal("wrong RPCGRPCStatusCodeKey")
	}
	if NetPeerNameKey != "net.peer.name" {
		t.Fatal("wrong NetPeerNameKey")
	}
}

func TestServerInterceptor_NilTracer(t *testing.T) {
	i := ServerInterceptor(nil)
	if i == nil {
		t.Fatal("should not be nil")
	}
	if i.Name() != "tracing" {
		t.Fatalf("Name = %q", i.Name())
	}
}

func TestClientInterceptor_NilTracer(t *testing.T) {
	i := ClientInterceptor(nil)
	if i == nil {
		t.Fatal("should not be nil")
	}
}

func TestInterceptor_Dependencies(t *testing.T) {
	i := ServerInterceptor(nil)
	deps := i.Dependencies()
	if len(deps) != 1 || deps[0] != "metadata" {
		t.Fatalf("Dependencies = %v", deps)
	}
}

func TestInterceptorName(t *testing.T) {
	if interceptorName != "tracing" {
		t.Fatal("wrong interceptorName")
	}
}
