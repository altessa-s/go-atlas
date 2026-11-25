// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache"
	"github.com/altessa-s/go-atlas/cache/storages/freecache"
	"github.com/altessa-s/go-atlas/cache/storages/lru"

	cachelru "github.com/altessa-s/go-atlas/cache/lru"
	nativefreecache "github.com/coocood/freecache"
)

type bytesSerializer struct{}

func (bytesSerializer) Serialize(value any) ([]byte, error) {
	return value.([]byte), nil
}

func (bytesSerializer) Deserialize(data []byte, value any) error {
	*value.(*[]byte) = bytes.Clone(data)
	return nil
}

func TestBinaryPayloads(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		payload []byte
	}{
		{"empty", []byte{}}, {"zero", []byte{0}}, {"two zeros", []byte{0, 0}}, {"zero prefix", []byte{0, 1}}, {"nonzero", []byte{1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			store, err := lru.New(8, cachelru.WithShardCount(1))
			require.NoError(t, err)
			c := cache.New(store, cache.WithSerializer(bytesSerializer{}), cache.WithNegativeTTL(time.Minute), cache.WithIsolatedResults())
			missing := func(context.Context) ([]byte, time.Duration, error) {
				return nil, cache.TTLUseDefault, cache.ErrMissing
			}
			_, err = cache.GetWithFallbackT(ctx, c, "key", missing)
			require.ErrorIs(t, err, cache.ErrMissing)
			exists, err := c.Exists(ctx, "key")
			require.NoError(t, err)
			require.False(t, exists)
			// Save replaces the negative entry.
			require.NoError(t, c.Save(ctx, "key", tc.payload))
			var got []byte
			require.NoError(t, c.Get(ctx, "key", &got))
			require.Equal(t, tc.payload, got)
			exists, err = c.Exists(ctx, "key")
			require.NoError(t, err)
			require.True(t, exists)
			// An isolated fill and a later hit both deliver unescaped bytes.
			got, err = cache.GetWithFallbackT(ctx, c, "fill", func(context.Context) ([]byte, time.Duration, error) {
				return tc.payload, cache.TTLUseDefault, nil
			})
			require.NoError(t, err)
			require.Equal(t, tc.payload, got)
			got, err = cache.GetWithFallbackT(ctx, c, "fill", missing)
			require.NoError(t, err)
			require.Equal(t, tc.payload, got)
			raw, err := c.GetManyRaw(ctx, []string{"fill", "key"})
			require.NoError(t, err)
			require.Equal(t, map[string][]byte{"fill": tc.payload, "key": tc.payload}, raw)
			require.NoError(t, c.DeleteMany(ctx, "key", "fill"))
			raw, err = c.GetManyRaw(ctx, []string{"fill", "key"})
			require.NoError(t, err)
			require.Empty(t, raw)
		})
	}
}

func TestLegacyCacheEntries(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	store, err := lru.New(8, cachelru.WithShardCount(1))
	require.NoError(t, err)
	require.NoError(t, store.Save(ctx, "json", []byte(`"cached"`), cache.NoTTL))
	require.NoError(t, store.Save(ctx, "negative", []byte{0}, cache.NoTTL))
	require.NoError(t, store.Save(ctx, "binary", []byte{0, 0}, cache.NoTTL))
	c := cache.New(store)
	var got string
	// The migration starts cold and leaves previous keys for expiry or cleanup.
	require.ErrorIs(t, c.Get(ctx, "json", &got), cache.ErrMissing)
	require.ErrorIs(t, c.Get(ctx, "negative", &got), cache.ErrMissing)
	require.NoError(t, c.Save(ctx, "json", "fresh"))
	require.NoError(t, c.Delete(ctx, "json"))
	legacy, err := store.Get(ctx, "json")
	require.NoError(t, err)
	require.Equal(t, []byte(`"cached"`), legacy)

	// A legacy binary entry might already contain the new escape sequence.
	binary := cache.New(store, cache.WithSerializer(bytesSerializer{}))
	var data []byte
	require.ErrorIs(t, binary.Get(ctx, "binary", &data), cache.ErrMissing)
}

func TestCacheKeyFormatsAreDisjoint(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	store := newLRU(t)
	json := cache.New(store)
	binary := cache.New(store, cache.WithSerializer(bytesSerializer{}))

	for _, jsonKey := range []string{"item", "\x00atlas-cache-v2:item", "\x00atlas-cache-v3:c0:item"} {
		require.NoError(t, json.Save(ctx, jsonKey, "json"))
	}
	require.NoError(t, binary.Save(ctx, "item", []byte{0}))
	raw, err := binary.GetManyRaw(ctx, []string{"item"})
	require.NoError(t, err)
	require.Equal(t, []byte{0}, raw["item"])
	require.NoError(t, binary.DeleteMany(ctx, "item"))
	for _, jsonKey := range []string{"item", "\x00atlas-cache-v2:item", "\x00atlas-cache-v3:c0:item"} {
		var got string
		require.NoError(t, json.Get(ctx, jsonKey, &got))
		require.Equal(t, "json", got)
	}
}

func TestCacheKeyComponentsAreDisjoint(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	store := newLRU(t)
	plain := cache.New(store)
	namespace := cache.WithKeyNamespace(func(context.Context) string { return "" })
	version := cache.WithKeyVersion(func(context.Context) (uint64, error) { return 0, nil })
	namespaced := cache.New(store, namespace)
	versioned := cache.New(store, version)
	both := cache.New(store, version, namespace)

	require.NoError(t, plain.Save(ctx, "0::item", "plain"))
	require.NoError(t, namespaced.Save(ctx, "item", "namespaced"))
	require.NoError(t, versioned.Save(ctx, ":item", "versioned"))
	require.NoError(t, versioned.Save(ctx, "0::item", "versioned-2"))
	require.NoError(t, both.Save(ctx, "item", "both"))
	for _, tc := range []struct {
		c        *cache.Cache
		key      string
		expected string
	}{
		{plain, "0::item", "plain"},
		{namespaced, "item", "namespaced"},
		{versioned, ":item", "versioned"},
		{versioned, "0::item", "versioned-2"},
		{both, "item", "both"},
	} {
		var got string
		require.NoError(t, tc.c.Get(ctx, tc.key, &got))
		require.Equal(t, tc.expected, got)
	}
}

func TestBinaryCacheSupportsMaximumFreeCacheKey(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		maxSize  int
		keyBytes int
	}{
		{name: "key length", maxSize: 32 << 20, keyBytes: 65535},
		{name: "entry size", maxSize: 1 << 20, keyBytes: 990},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := cache.New(freecache.New(freecache.WithMaxSize(tc.maxSize)), cache.WithSerializer(bytesSerializer{}))
			key := strings.Repeat("x", tc.keyBytes)
			require.NoError(t, c.Save(t.Context(), key, []byte{0}))
			found, err := c.Exists(t.Context(), key)
			require.NoError(t, err)
			require.True(t, found)
			raw, err := c.GetManyRaw(t.Context(), []string{key})
			require.NoError(t, err)
			require.Equal(t, []byte{0}, raw[key])
			require.NoError(t, c.DeleteMany(t.Context(), key))
			var got []byte
			require.ErrorIs(t, c.Get(t.Context(), key, &got), cache.ErrMissing)
		})
	}
}

func TestFreeCacheShortKeyEntryBudget(t *testing.T) {
	t.Parallel()
	c := cache.New(freecache.New(freecache.WithMaxSize(1 << 20)))
	// The 1000-byte budget includes the format tag, key and JSON quotes.
	for _, size := range []int{988, 996} {
		value := strings.Repeat("a", size)
		require.NoError(t, c.Save(t.Context(), "x", value))
		var got string
		require.NoError(t, c.Get(t.Context(), "x", &got))
		require.Equal(t, value, got)
	}
	require.ErrorIs(t, c.Save(t.Context(), "x", strings.Repeat("a", 997)), nativefreecache.ErrLargeEntry)
}
