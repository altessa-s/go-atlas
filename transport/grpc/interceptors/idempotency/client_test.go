// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestDeriveKey_DeterministicAndValid(t *testing.T) {
	k1 := DeriveKey("operation-1", "svc.Service/Call")
	k2 := DeriveKey("operation-1", "svc.Service/Call")
	if k1 != k2 {
		t.Fatalf("DeriveKey is not deterministic: %q != %q", k1, k2)
	}
	if err := DefaultKeyValidator(k1); err != nil {
		t.Fatalf("derived key %q is not a valid lowercase UUID v4: %v", k1, err)
	}
}

func TestDeriveKey_DistinctPerSeedAndCall(t *testing.T) {
	base := DeriveKey("operation-1", "svc.Service/Call")
	if got := DeriveKey("operation-2", "svc.Service/Call"); got == base {
		t.Fatal("a different seed must yield a different key")
	}
	if got := DeriveKey("operation-1", "svc.Service/Other"); got == base {
		t.Fatal("a different call must yield a different key")
	}
}

func TestWithKey_AttachesSingleHeaderValue(t *testing.T) {
	const key = "3b24398f-257d-4879-8ba1-89cb176abe72"

	ctx := WithKey(context.Background(), key)

	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata to be present")
	}
	if got := md.Get(DefaultIdempotencyKeyHeader); len(got) != 1 || got[0] != key {
		t.Fatalf("unexpected %s header: %v", DefaultIdempotencyKeyHeader, got)
	}
}

func TestWithKey_PreservesExistingMetadata(t *testing.T) {
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer t")

	ctx = WithKey(ctx, "3b24398f-257d-4879-8ba1-89cb176abe72")

	md, _ := metadata.FromOutgoingContext(ctx)
	if got := md.Get("authorization"); len(got) != 1 || got[0] != "Bearer t" {
		t.Fatalf("existing metadata not preserved: %v", got)
	}
}

func TestWithDerivedKey_AttachesValidHeader(t *testing.T) {
	ctx := WithDerivedKey(context.Background(), "operation-1", "svc.Service/Call")

	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata to be present")
	}
	got := md.Get(DefaultIdempotencyKeyHeader)
	if len(got) != 1 {
		t.Fatalf("expected exactly one header value, got %v", got)
	}
	if err := DefaultKeyValidator(got[0]); err != nil {
		t.Fatalf("attached key %q is invalid: %v", got[0], err)
	}
}
