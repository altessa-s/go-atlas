// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metadata

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"

	"google.golang.org/grpc"
)

func TestNewCallMetadata_UnaryServer(t *testing.T) {
	ctx := t.Context()
	info := &grpc.UnaryServerInfo{FullMethod: "/mypackage.MyService/MyMethod"}
	meta := NewCallMetadata(ctx, info.FullMethod, info)

	require.Equal(t, "mypackage.MyService", meta.ServiceName)
	require.Equal(t, "MyMethod", meta.MethodName)
	require.Equal(t, "/mypackage.MyService/MyMethod", meta.FullyMethodName)
	require.False(t, meta.IsClient, "should not be client")
	require.False(t, meta.IsStream, "should not be stream")
	require.False(t, meta.StartTime.IsZero(), "StartTime should be set")
}

func TestNewCallMetadata_StreamServer(t *testing.T) {
	ctx := t.Context()
	info := &grpc.StreamServerInfo{
		FullMethod:     "/pkg.Svc/Stream",
		IsClientStream: true,
		IsServerStream: false,
	}
	meta := NewCallMetadata(ctx, info.FullMethod, info)

	require.True(t, meta.IsStream, "should be stream")
	require.Equal(t, driver.StreamTypeClient, meta.StreamType)
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
			require.Equal(t, tt.want, meta.StreamType)
		})
	}
}

func TestNewCallMetadataFromMethod(t *testing.T) {
	meta := NewCallMetadataFromMethod(t.Context(), "/pkg.Svc/GetUser")
	require.Equal(t, "pkg.Svc", meta.ServiceName)
	require.Equal(t, "GetUser", meta.MethodName)
	require.True(t, meta.IsClient, "should be client")
}

func TestCallMetadata_Duration(t *testing.T) {
	meta := &CallMetadata{}
	require.EqualValues(t, 0, meta.Duration())
}

func TestContext_RoundTrip(t *testing.T) {
	ctx := t.Context()
	_, ok := FromContext(ctx)
	require.False(t, ok, "should not find metadata in empty context")

	meta := &CallMetadata{ServiceName: "test"}
	ctx = NewContext(ctx, meta)
	got, ok := FromContext(ctx)
	require.True(t, ok, "should find metadata")
	require.Equal(t, "test", got.ServiceName)
}

func TestEnsureInContext_New(t *testing.T) {
	ctx := t.Context()
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Method"}
	ctx, meta := EnsureInContext(ctx, info.FullMethod, info)

	require.Equal(t, "pkg.Svc", meta.ServiceName)

	// Second call should return same metadata
	_, meta2 := EnsureInContext(ctx, info.FullMethod, info)
	require.Equal(t, meta, meta2)
}

func TestEnsureInContextFromMethod(t *testing.T) {
	ctx := t.Context()
	ctx, meta := EnsureInContextFromMethod(ctx, "/pkg.Svc/Method")

	require.Equal(t, "pkg.Svc", meta.ServiceName)

	// Second call should return same
	_, meta2 := EnsureInContextFromMethod(ctx, "/pkg.Svc/Method")
	require.Equal(t, meta, meta2)
}

func TestNewCallMetadata_NoSlash(t *testing.T) {
	meta := NewCallMetadataFromMethod(t.Context(), "NoSlashMethod")
	require.Equal(t, "NoSlashMethod", meta.ServiceName)
	require.Equal(t, "", meta.MethodName)
}

func TestNewCallMetadata_MethodCache(t *testing.T) {
	ctx := t.Context()
	method := "/cache.Test/CacheMethod"
	info := &grpc.UnaryServerInfo{FullMethod: method}

	meta1 := NewCallMetadata(ctx, method, info)
	meta2 := NewCallMetadata(ctx, method, info)

	require.Equal(t, meta2.ServiceName, meta1.ServiceName)
}
