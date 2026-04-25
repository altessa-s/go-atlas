// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

import (
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/metadata"
)

func TestMetadataPool(t *testing.T) {
	md := GetMetadata()
	require.NotNil(t, md, "should not be nil")
	require.Len(t, md, 0)

	md.Set("key", "value")
	PutMetadata(md)

	// Get again - should be cleared
	md2 := GetMetadata()
	require.Len(t, md2, 0)
}

func TestMetadataPool_OversizedNotReturned(t *testing.T) {
	md := GetMetadata()
	for i := 0; i < MaxMetadataPoolSize+1; i++ {
		md.Set("key"+string(rune('a'+i)), "value")
	}
	// Should not panic
	PutMetadata(md)
}

func TestGrpcHeaderGetter(t *testing.T) {
	md := metadata.Pairs("x-request-id", "abc-123")
	g := &grpcHeaderGetter{md: md}

	got := g.GetHeader("x-request-id")
	require.Equal(t, "abc-123", got)
	got = g.GetHeader("nonexistent")
	require.Equal(t, "", got)
}

func TestGrpcHeaderGetter_NilMD(t *testing.T) {
	g := &grpcHeaderGetter{md: nil}
	got := g.GetHeader("any")
	require.Equal(t, "", got)
}

func TestContext_RoundTrip(t *testing.T) {
	ctx := t.Context()
	got := FromContext(ctx)
	require.Equal(t, "", got)

	ctx = NewContext(ctx, "test-id")
	got = FromContext(ctx)
	require.Equal(t, "test-id", got)
}

func TestServerInterceptor_Dependencies(t *testing.T) {
	i := &interceptor{}
	deps := i.Dependencies()
	require.Len(t, deps, 1)
	require.Equal(t, "metadata", deps[0])
}

func BenchmarkGetPutMetadata(b *testing.B) {
	for b.Loop() {
		md := GetMetadata()
		md.Set("key", "value")
		PutMetadata(md)
	}
}

func BenchmarkGrpcHeaderGetter_GetHeader(b *testing.B) {
	md := metadata.Pairs("x-request-id", "abc-123")
	g := &grpcHeaderGetter{md: md}
	for b.Loop() {
		g.GetHeader("x-request-id")
	}
}
