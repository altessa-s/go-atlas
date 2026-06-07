// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"

	"google.golang.org/grpc/metadata"
)

func TestDeriveKey_DeterministicAndValid(t *testing.T) {
	t.Parallel()

	k1 := idempotency.DeriveKey("operation-1", "svc.Service/Call")
	k2 := idempotency.DeriveKey("operation-1", "svc.Service/Call")
	require.Equal(t, k1, k2, "DeriveKey must be deterministic")
	require.NoError(t, idempotency.DefaultKeyValidator(k1), "derived key must satisfy DefaultKeyValidator")
}

func TestDeriveKey_DistinctPerSeedAndCall(t *testing.T) {
	t.Parallel()

	base := idempotency.DeriveKey("operation-1", "svc.Service/Call")
	require.NotEqual(t, base, idempotency.DeriveKey("operation-2", "svc.Service/Call"), "a different seed must yield a different key")
	require.NotEqual(t, base, idempotency.DeriveKey("operation-1", "svc.Service/Other"), "a different call must yield a different key")
}

func TestWithKey_AttachesSingleHeaderValue(t *testing.T) {
	t.Parallel()

	const key = "3b24398f-257d-4879-8ba1-89cb176abe72"

	ctx := idempotency.WithKey(t.Context(), key)

	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok, "outgoing metadata must be present")
	require.Equal(t, []string{key}, md.Get(idempotency.DefaultIdempotencyKeyHeader))
}

func TestWithKey_PreservesExistingMetadata(t *testing.T) {
	t.Parallel()

	ctx := metadata.AppendToOutgoingContext(t.Context(), "authorization", "Bearer t")

	ctx = idempotency.WithKey(ctx, "3b24398f-257d-4879-8ba1-89cb176abe72")

	md, _ := metadata.FromOutgoingContext(ctx)
	require.Equal(t, []string{"Bearer t"}, md.Get("authorization"), "existing metadata must be preserved")
}

func TestWithDerivedKey_AttachesValidHeader(t *testing.T) {
	t.Parallel()

	ctx := idempotency.WithDerivedKey(t.Context(), "operation-1", "svc.Service/Call")

	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok, "outgoing metadata must be present")
	got := md.Get(idempotency.DefaultIdempotencyKeyHeader)
	require.Len(t, got, 1, "expected exactly one header value")
	require.NoError(t, idempotency.DefaultKeyValidator(got[0]), "attached key must satisfy DefaultKeyValidator")
}
