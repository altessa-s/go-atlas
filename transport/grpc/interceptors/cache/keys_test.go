// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	grpcmetadata "google.golang.org/grpc/metadata"
)

func TestDefaultKeyGenerator(t *testing.T) {
	ctx := t.Context()
	key, err := DefaultKeyGenerator(ctx, "/svc/Get", "req")
	require.NoError(t, err)
	require.NotEqual(t, "", key)
	require.Len(t, key, 16)
}

func TestDefaultKeyGenerator_Deterministic(t *testing.T) {
	ctx := t.Context()
	k1, _ := DefaultKeyGenerator(ctx, "/svc/Get", "req")
	k2, _ := DefaultKeyGenerator(ctx, "/svc/Get", "req")
	require.Equal(t, k2, k1)
}

func TestDefaultKeyGenerator_DifferentRequests(t *testing.T) {
	ctx := t.Context()
	k1, _ := DefaultKeyGenerator(ctx, "/svc/Get", "req1")
	k2, _ := DefaultKeyGenerator(ctx, "/svc/Get", "req2")
	require.NotEqual(t, k2, k1)
}

func TestDefaultKeyGenerator_DifferentMethods(t *testing.T) {
	ctx := t.Context()
	k1, _ := DefaultKeyGenerator(ctx, "/svc/Get", "req")
	k2, _ := DefaultKeyGenerator(ctx, "/svc/List", "req")
	require.NotEqual(t, k2, k1)
}

func TestNewKeyGenerator_WithMetadata(t *testing.T) {
	gen := NewKeyGenerator([]string{"user-id"}, nil)
	md := grpcmetadata.Pairs("user-id", "u1")
	ctx1 := grpcmetadata.NewIncomingContext(t.Context(), md)
	ctx2 := grpcmetadata.NewIncomingContext(t.Context(), grpcmetadata.Pairs("user-id", "u2"))

	k1, _ := gen(ctx1, "/svc/Get", "req")
	k2, _ := gen(ctx2, "/svc/Get", "req")
	require.NotEqual(t, k2, k1)
}

func TestNewKeyGenerator_WithProcessor(t *testing.T) {
	processor := func(_ context.Context, md grpcmetadata.MD) map[string]string {
		return map[string]string{"custom": "val"}
	}
	gen := NewKeyGenerator(nil, processor)
	ctx := grpcmetadata.NewIncomingContext(t.Context(), grpcmetadata.MD{})
	key, err := gen(ctx, "/svc/Get", "req")
	require.NoError(t, err)
	require.NotEqual(t, "", key)
}

func TestDefaultMetadataKeys(t *testing.T) {
	require.NotEqual(t, 0, len(DefaultMetadataKeys))
}
