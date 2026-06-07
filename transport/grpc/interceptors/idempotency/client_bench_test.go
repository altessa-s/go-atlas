// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"

	"google.golang.org/grpc/metadata"
)

func BenchmarkDeriveKey(b *testing.B) {
	const (
		seed = "operation-3b24398f-257d-4879-8ba1-89cb176abe72"
		call = "users.UserService/Update"
	)

	b.ReportAllocs()
	for b.Loop() {
		_ = idempotency.DeriveKey(seed, call)
	}
}

func BenchmarkWithKey_NoExistingMetadata(b *testing.B) {
	const key = "3b24398f-257d-4879-8ba1-89cb176abe72"
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		_ = idempotency.WithKey(ctx, key)
	}
}

func BenchmarkWithKey_WithExistingMetadata(b *testing.B) {
	const key = "3b24398f-257d-4879-8ba1-89cb176abe72"
	ctx := metadata.AppendToOutgoingContext(context.Background(),
		"authorization", "Bearer t",
		"x-request-id", "11111111-1111-1111-1111-111111111111",
	)

	b.ReportAllocs()
	for b.Loop() {
		_ = idempotency.WithKey(ctx, key)
	}
}

func BenchmarkWithDerivedKey(b *testing.B) {
	const (
		seed = "operation-3b24398f-257d-4879-8ba1-89cb176abe72"
		call = "users.UserService/Update"
	)
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		_ = idempotency.WithDerivedKey(ctx, seed, call)
	}
}
