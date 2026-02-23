// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metadata

import (
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"

	"google.golang.org/grpc"
)

func TestNewCallMetadata_UnaryServer(t *testing.T) {
	ctx := t.Context()
	info := &grpc.UnaryServerInfo{FullMethod: "/mypackage.MyService/MyMethod"}
	meta := NewCallMetadata(ctx, info.FullMethod, info)

	if meta.ServiceName != "mypackage.MyService" {
		t.Fatalf("ServiceName = %q", meta.ServiceName)
	}
	if meta.MethodName != "MyMethod" {
		t.Fatalf("MethodName = %q", meta.MethodName)
	}
	if meta.FullyMethodName != "/mypackage.MyService/MyMethod" {
		t.Fatalf("FullyMethodName = %q", meta.FullyMethodName)
	}
	if meta.IsClient {
		t.Fatal("should not be client")
	}
	if meta.IsStream {
		t.Fatal("should not be stream")
	}
	if meta.StartTime.IsZero() {
		t.Fatal("StartTime should be set")
	}
}

func TestNewCallMetadata_StreamServer(t *testing.T) {
	ctx := t.Context()
	info := &grpc.StreamServerInfo{
		FullMethod:     "/pkg.Svc/Stream",
		IsClientStream: true,
		IsServerStream: false,
	}
	meta := NewCallMetadata(ctx, info.FullMethod, info)

	if !meta.IsStream {
		t.Fatal("should be stream")
	}
	if meta.StreamType != driver.StreamTypeClient {
		t.Fatalf("StreamType = %v, want client", meta.StreamType)
	}
}

func TestNewCallMetadata_StreamDesc(t *testing.T) {
	tests := []struct {
		name string
		desc *grpc.StreamDesc
		want driver.StreamType
	}{
		{"client_stream", &grpc.StreamDesc{ClientStreams: true, ServerStreams: false}, driver.StreamTypeClient},
		{"server_stream", &grpc.StreamDesc{ClientStreams: false, ServerStreams: true}, driver.StreamTypeServer},
		{"bidi", &grpc.StreamDesc{ClientStreams: true, ServerStreams: true}, driver.StreamTypeBidi},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := NewCallMetadata(t.Context(), "/pkg.Svc/Method", tt.desc)
			if meta.StreamType != tt.want {
				t.Fatalf("StreamType = %v, want %v", meta.StreamType, tt.want)
			}
		})
	}
}

func TestNewCallMetadataFromMethod(t *testing.T) {
	meta := NewCallMetadataFromMethod(t.Context(), "/pkg.Svc/GetUser")
	if meta.ServiceName != "pkg.Svc" {
		t.Fatalf("ServiceName = %q", meta.ServiceName)
	}
	if meta.MethodName != "GetUser" {
		t.Fatalf("MethodName = %q", meta.MethodName)
	}
	if !meta.IsClient {
		t.Fatal("should be client")
	}
}

func TestCallMetadata_Duration(t *testing.T) {
	meta := &CallMetadata{}
	if meta.Duration() != 0 {
		t.Fatal("zero start time should return 0 duration")
	}
}

func TestContext_RoundTrip(t *testing.T) {
	ctx := t.Context()
	_, ok := FromContext(ctx)
	if ok {
		t.Fatal("should not find metadata in empty context")
	}

	meta := &CallMetadata{ServiceName: "test"}
	ctx = NewContext(ctx, meta)
	got, ok := FromContext(ctx)
	if !ok {
		t.Fatal("should find metadata")
	}
	if got.ServiceName != "test" {
		t.Fatalf("ServiceName = %q", got.ServiceName)
	}
}

func TestEnsureInContext_New(t *testing.T) {
	ctx := t.Context()
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Method"}
	ctx, meta := EnsureInContext(ctx, info.FullMethod, info)

	if meta.ServiceName != "pkg.Svc" {
		t.Fatalf("ServiceName = %q", meta.ServiceName)
	}

	// Second call should return same metadata
	_, meta2 := EnsureInContext(ctx, info.FullMethod, info)
	if meta2 != meta {
		t.Fatal("should return same metadata on second call")
	}
}

func TestEnsureInContextFromMethod(t *testing.T) {
	ctx := t.Context()
	ctx, meta := EnsureInContextFromMethod(ctx, "/pkg.Svc/Method")

	if meta.ServiceName != "pkg.Svc" {
		t.Fatalf("ServiceName = %q", meta.ServiceName)
	}

	// Second call should return same
	_, meta2 := EnsureInContextFromMethod(ctx, "/pkg.Svc/Method")
	if meta2 != meta {
		t.Fatal("should return same metadata")
	}
}

func TestNewCallMetadata_NoSlash(t *testing.T) {
	meta := NewCallMetadataFromMethod(t.Context(), "NoSlashMethod")
	if meta.ServiceName != "NoSlashMethod" {
		t.Fatalf("ServiceName = %q", meta.ServiceName)
	}
	if meta.MethodName != "" {
		t.Fatalf("MethodName = %q, want empty", meta.MethodName)
	}
}

func TestNewCallMetadata_MethodCache(t *testing.T) {
	ctx := t.Context()
	method := "/cache.Test/CacheMethod"
	info := &grpc.UnaryServerInfo{FullMethod: method}

	meta1 := NewCallMetadata(ctx, method, info)
	meta2 := NewCallMetadata(ctx, method, info)

	if meta1.ServiceName != meta2.ServiceName {
		t.Fatal("cached results should match")
	}
}
