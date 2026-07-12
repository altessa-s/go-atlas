// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/cache/providers"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/wrapperspb"

	datacache "github.com/altessa-s/go-atlas/data/cache"
	grpcmetadata "google.golang.org/grpc/metadata"
)

// benchMemoryProvider is a minimal in-memory cache provider for benchmarks,
// mirroring the map-backed provider used by data/cache benchmarks.
type benchMemoryProvider struct {
	store map[string][]byte
}

func (p *benchMemoryProvider) Save(_ context.Context, key string, value []byte, _ time.Duration) error {
	p.store[key] = value
	return nil
}

func (p *benchMemoryProvider) Get(_ context.Context, key string) ([]byte, error) {
	v, ok := p.store[key]
	if !ok {
		return nil, providers.ErrMissing
	}
	return v, nil
}

func (p *benchMemoryProvider) Delete(_ context.Context, _ string) error         { return nil }
func (p *benchMemoryProvider) DeleteMany(_ context.Context, _ ...string) error  { return nil }
func (p *benchMemoryProvider) Exists(_ context.Context, _ string) (bool, error) { return true, nil }

// BenchmarkKeyGenerator_WithMetadata measures the cache key build path with
// caller-identity metadata present, the common shape in production traffic.
func BenchmarkKeyGenerator_WithMetadata(b *testing.B) {
	gen := NewKeyGenerator([]string{"user-id", "tenant-id"}, nil)
	md := grpcmetadata.Pairs("user-id", "u1", "tenant-id", "t1")
	ctx := grpcmetadata.NewIncomingContext(b.Context(), md)
	req := wrapperspb.String("payload")

	b.ReportAllocs()
	for b.Loop() {
		if _, err := gen(ctx, "/bench.Service/Get", req); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkServerInterceptor_CacheHit measures the full unary interceptor hit
// path: key generation, in-memory lookup, and cached response deserialization.
func BenchmarkServerInterceptor_CacheHit(b *testing.B) {
	cacher := datacache.New(&benchMemoryProvider{store: make(map[string][]byte)})
	si := ServerInterceptor(cacher,
		WithMethod("/bench.Service/Get", &wrapperspb.StringValue{}),
		WithCacheHeaders(false),
	)
	unary := si.ServerUnaryInterceptor()

	info := &grpc.UnaryServerInfo{FullMethod: "/bench.Service/Get"}
	req := wrapperspb.String("request")
	resp := wrapperspb.String(strings.Repeat("cached response payload ", 8))
	handlerCalls := 0
	handler := func(context.Context, any) (any, error) {
		handlerCalls++
		return resp, nil
	}
	ctx := b.Context()

	// Warm the cache: the first call is a miss and populates the entry.
	if _, err := unary(ctx, req, info, handler); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := unary(ctx, req, info, handler); err != nil {
			b.Fatal(err)
		}
	}

	if handlerCalls != 1 {
		b.Fatalf("expected cache hits after warm-up, handler invoked %d times", handlerCalls)
	}
}

// BenchmarkDefaultKeyGenerator measures the bare key build path without
// identity metadata.
func BenchmarkDefaultKeyGenerator(b *testing.B) {
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		DefaultKeyGenerator(ctx, "/svc/Get", "req") //nolint:errcheck
	}
}

// BenchmarkDefaultSuccessOnlyDecision measures the per-response caching
// decision on the success path.
func BenchmarkDefaultSuccessOnlyDecision(b *testing.B) {
	fn := DefaultSuccessOnlyDecision(5 * time.Minute)
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		fn(ctx, "/svc/Method", nil, "resp", nil)
	}
}
