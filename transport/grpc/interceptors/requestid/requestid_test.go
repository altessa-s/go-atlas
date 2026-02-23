// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

import (
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestMetadataPool(t *testing.T) {
	md := GetMetadata()
	if md == nil {
		t.Fatal("should not be nil")
	}
	if len(md) != 0 {
		t.Fatalf("should be empty, got %d", len(md))
	}

	md.Set("key", "value")
	PutMetadata(md)

	// Get again - should be cleared
	md2 := GetMetadata()
	if len(md2) != 0 {
		t.Fatalf("should be empty after pool return, got %d", len(md2))
	}
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

	if got := g.GetHeader("x-request-id"); got != "abc-123" {
		t.Fatalf("GetHeader = %q", got)
	}
	if got := g.GetHeader("nonexistent"); got != "" {
		t.Fatalf("GetHeader nonexistent = %q", got)
	}
}

func TestGrpcHeaderGetter_NilMD(t *testing.T) {
	g := &grpcHeaderGetter{md: nil}
	if got := g.GetHeader("any"); got != "" {
		t.Fatalf("GetHeader nil md = %q", got)
	}
}

func TestContext_RoundTrip(t *testing.T) {
	ctx := t.Context()
	if got := FromContext(ctx); got != "" {
		t.Fatalf("FromContext empty = %q", got)
	}

	ctx = NewContext(ctx, "test-id")
	if got := FromContext(ctx); got != "test-id" {
		t.Fatalf("FromContext = %q", got)
	}
}

func TestErrInvalidRequestId(t *testing.T) {
	if ErrInvalidRequestId == nil {
		t.Fatal("should not be nil")
	}
}

func TestMaxMetadataPoolSize(t *testing.T) {
	if MaxMetadataPoolSize != 10 {
		t.Fatalf("MaxMetadataPoolSize = %d", MaxMetadataPoolSize)
	}
}

func TestServerInterceptor_Dependencies(t *testing.T) {
	i := &interceptor{}
	deps := i.Dependencies()
	if len(deps) != 1 || deps[0] != "metadata" {
		t.Fatalf("Dependencies = %v", deps)
	}
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
