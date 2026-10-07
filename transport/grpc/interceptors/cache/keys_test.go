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

// TestKeyGenerator_FramingPreventsConcatenationCollisions pins that key
// components are length-framed: before the fix these pairs hashed the same
// byte stream (e.g. "tenant-idauser-idb") and shared a cache entry.
func TestKeyGenerator_FramingPreventsConcatenationCollisions(t *testing.T) {
	t.Parallel()

	processor := func(_ context.Context, md grpcmetadata.MD) map[string]string {
		extra := map[string]string{}
		for k, v := range md {
			if len(k) > 2 && k[:2] == "p-" {
				extra[k[2:]] = v[0]
			}
		}
		return extra
	}
	gen := NewKeyGenerator(nil, processor)

	for _, tc := range []struct {
		name string
		a, b grpcmetadata.MD
	}{
		{
			name: "value absorbs next key",
			a:    grpcmetadata.Pairs("tenant-id", "auser-idb"),
			b:    grpcmetadata.Pairs("tenant-id", "a", "user-id", "b"),
		},
		{
			name: "multi-value split",
			a:    grpcmetadata.MD{"tenant-id": {"ab", "c"}},
			b:    grpcmetadata.MD{"tenant-id": {"a", "bc"}},
		},
		{
			name: "processed key/value split",
			a:    grpcmetadata.Pairs("p-a", "bc"),
			b:    grpcmetadata.Pairs("p-ab", "c"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ka, err := gen(grpcmetadata.NewIncomingContext(t.Context(), tc.a), "/svc/Get", "req")
			require.NoError(t, err)
			kb, err := gen(grpcmetadata.NewIncomingContext(t.Context(), tc.b), "/svc/Get", "req")
			require.NoError(t, err)
			require.NotEqual(t, ka, kb)
		})
	}
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
