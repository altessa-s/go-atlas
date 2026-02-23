// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"testing"

	grpcmetadata "google.golang.org/grpc/metadata"
)

func TestDefaultKeyGenerator(t *testing.T) {
	ctx := t.Context()
	key, err := DefaultKeyGenerator(ctx, "/svc/Get", "req")
	if err != nil {
		t.Fatal(err)
	}
	if key == "" {
		t.Fatal("expected non-empty key")
	}
	if len(key) != 16 {
		t.Fatalf("key length = %d, expected 16 hex chars", len(key))
	}
}

func TestDefaultKeyGenerator_Deterministic(t *testing.T) {
	ctx := t.Context()
	k1, _ := DefaultKeyGenerator(ctx, "/svc/Get", "req")
	k2, _ := DefaultKeyGenerator(ctx, "/svc/Get", "req")
	if k1 != k2 {
		t.Fatal("same input should produce same key")
	}
}

func TestDefaultKeyGenerator_DifferentRequests(t *testing.T) {
	ctx := t.Context()
	k1, _ := DefaultKeyGenerator(ctx, "/svc/Get", "req1")
	k2, _ := DefaultKeyGenerator(ctx, "/svc/Get", "req2")
	if k1 == k2 {
		t.Fatal("different requests should produce different keys")
	}
}

func TestDefaultKeyGenerator_DifferentMethods(t *testing.T) {
	ctx := t.Context()
	k1, _ := DefaultKeyGenerator(ctx, "/svc/Get", "req")
	k2, _ := DefaultKeyGenerator(ctx, "/svc/List", "req")
	if k1 == k2 {
		t.Fatal("different methods should produce different keys")
	}
}

func TestNewKeyGenerator_WithMetadata(t *testing.T) {
	gen := NewKeyGenerator([]string{"user-id"}, nil)
	md := grpcmetadata.Pairs("user-id", "u1")
	ctx1 := grpcmetadata.NewIncomingContext(t.Context(), md)
	ctx2 := grpcmetadata.NewIncomingContext(t.Context(), grpcmetadata.Pairs("user-id", "u2"))

	k1, _ := gen(ctx1, "/svc/Get", "req")
	k2, _ := gen(ctx2, "/svc/Get", "req")
	if k1 == k2 {
		t.Fatal("different metadata should produce different keys")
	}
}

func TestNewKeyGenerator_WithProcessor(t *testing.T) {
	processor := func(_ context.Context, md grpcmetadata.MD) map[string]string {
		return map[string]string{"custom": "val"}
	}
	gen := NewKeyGenerator(nil, processor)
	ctx := grpcmetadata.NewIncomingContext(t.Context(), grpcmetadata.MD{})
	key, err := gen(ctx, "/svc/Get", "req")
	if err != nil {
		t.Fatal(err)
	}
	if key == "" {
		t.Fatal("expected non-empty key")
	}
}

func TestDefaultMetadataKeys(t *testing.T) {
	if len(DefaultMetadataKeys) == 0 {
		t.Fatal("DefaultMetadataKeys should not be empty")
	}
}

func BenchmarkDefaultKeyGenerator(b *testing.B) {
	ctx := b.Context()
	for b.Loop() {
		DefaultKeyGenerator(ctx, "/svc/Get", "req") //nolint:errcheck
	}
}
