// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestKeyGeneratorPreservesMethodCase(t *testing.T) {
	t.Parallel()
	gen := cache.NewKeyGenerator(nil, nil)
	a, err := gen(t.Context(), "/svc/Get", "request")
	require.NoError(t, err)
	b, err := gen(t.Context(), "/svc/get", "request")
	require.NoError(t, err)
	require.NotEqual(t, a, b)
}

func TestKeyGeneratorProtoMapsAreDeterministic(t *testing.T) {
	t.Parallel()
	req, err := structpb.NewStruct(map[string]any{"a": 1, "b": 2, "c": 3, "d": 4})
	require.NoError(t, err)
	gen := cache.NewKeyGenerator(nil, nil)
	want, err := gen(t.Context(), "/svc/Get", req)
	require.NoError(t, err)
	for range 100 {
		got, err := gen(t.Context(), "/svc/Get", req)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}

func TestKeyGeneratorRejectsOversizedIdentity(t *testing.T) {
	t.Parallel()
	gen := cache.NewKeyGenerator(nil, nil)
	ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("tenant-id", strings.Repeat("a", 1025)))
	_, err := gen(ctx, "/svc/Get", "request")
	require.ErrorIs(t, err, cache.ErrMetadataTooLarge)
}

func TestKeyGeneratorRejectsExcessMetadata(t *testing.T) {
	t.Parallel()
	manyKeys := make([]string, 101)
	manyMD := metadata.MD{}
	for i := range manyKeys {
		manyKeys[i] = fmt.Sprintf("key-%d", i)
		manyMD[manyKeys[i]] = []string{"value"}
	}
	longKey := strings.Repeat("k", 257)
	for _, tc := range []struct {
		name      string
		keys      []string
		md        metadata.MD
		processor cache.MetadataProcessor
	}{
		{name: "keys", keys: manyKeys, md: manyMD},
		{name: "key length", keys: []string{longKey}, md: metadata.Pairs(longKey, "value")},
		{name: "values", md: metadata.MD{"tenant-id": make([]string, 21)}},
		{name: "processor value", md: metadata.MD{}, processor: func(context.Context, metadata.MD) map[string]string {
			return map[string]string{"tenant": strings.Repeat("v", 1025)}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gen := cache.NewKeyGenerator(tc.keys, tc.processor)
			_, err := gen(metadata.NewIncomingContext(t.Context(), tc.md), "/svc/Get", "request")
			require.ErrorIs(t, err, cache.ErrMetadataTooLarge)
		})
	}
}

func TestKeyGeneratorPreservesProcessorKeyCase(t *testing.T) {
	t.Parallel()
	ctx := metadata.NewIncomingContext(t.Context(), metadata.MD{})
	key := func(name string) string {
		gen := cache.NewKeyGenerator(nil, func(context.Context, metadata.MD) map[string]string { return map[string]string{name: "value"} })
		got, err := gen(ctx, "/svc/Get", "request")
		require.NoError(t, err)
		return got
	}
	require.NotEqual(t, key("Tenant"), key("tenant"))
}

func TestNewKeyGeneratorUsesInterceptorIdentity(t *testing.T) {
	t.Parallel()

	type credential struct{ ID string }
	cacher := newMapCacher()
	unary := cache.ServerInterceptor(cacher,
		cache.WithMethod(echoMethod, &wrapperspb.StringValue{}),
		cache.WithKeyGenerator(cache.NewKeyGenerator(nil, nil)),
		cache.WithIdentityFunc(func(ctx context.Context) (string, cache.IdentityState) {
			creds, _ := auth.CredentialsFromContext(ctx)
			return creds.Data.(credential).ID, cache.Verified
		}),
	).ServerUnaryInterceptor()
	require.Equal(t, "alice", callWithCacheCredentials(t, t.Context(), unary, credential{"alice"}, "alice"))
	require.Equal(t, "bob", callWithCacheCredentials(t, t.Context(), unary, credential{"bob"}, "bob"))
	require.Equal(t, "alice", callWithCacheCredentials(t, t.Context(), unary, credential{"alice"}, "unexpected miss"))
	require.Len(t, cacher.keys(), 2)
	require.Equal(t, 2, cacher.saves)
}
