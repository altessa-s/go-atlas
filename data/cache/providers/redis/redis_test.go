// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache/providers"
	"github.com/altessa-s/go-atlas/data/cache/providers/redis"

	goredis "github.com/redis/go-redis/v9"
)

// setupProvider creates a Provider with a miniredis instance for testing.
func setupProvider(t *testing.T) (*redis.Provider, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	t.Cleanup(func() {
		mr.Close()
	})

	client := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	provider := redis.New(client)
	return provider, mr
}

func TestNew(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	provider := redis.New(client)
	require.NotNil(t, provider)
}

func TestNew_WithPrefix(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	provider := redis.New(client, redis.WithPrefix("test:"))
	require.NotNil(t, provider)
}

func TestProvider_SaveAndGet(t *testing.T) {
	provider, _ := setupProvider(t)
	ctx := t.Context()

	key := "test-key"
	value := []byte("test-value")

	err := provider.Save(ctx, key, value, 0)
	require.NoError(t, err)

	got, err := provider.Get(ctx, key)
	require.NoError(t, err)
	require.Equal(t, string(value), string(got))
}

func TestProvider_Get_Missing(t *testing.T) {
	provider, _ := setupProvider(t)
	ctx := t.Context()

	_, err := provider.Get(ctx, "nonexistent-key")
	require.ErrorIs(t, err, providers.ErrMissing)
}

func TestProvider_Exists_True(t *testing.T) {
	provider, _ := setupProvider(t)
	ctx := t.Context()

	key := "test-key"
	value := []byte("test-value")

	err := provider.Save(ctx, key, value, 0)
	require.NoError(t, err)

	exists, err := provider.Exists(ctx, key)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestProvider_Exists_False(t *testing.T) {
	provider, _ := setupProvider(t)
	ctx := t.Context()

	exists, err := provider.Exists(ctx, "nonexistent-key")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestProvider_Delete(t *testing.T) {
	provider, _ := setupProvider(t)
	ctx := t.Context()

	key := "test-key"
	value := []byte("test-value")

	err := provider.Save(ctx, key, value, 0)
	require.NoError(t, err)

	err = provider.Delete(ctx, key)
	require.NoError(t, err)

	_, err = provider.Get(ctx, key)
	require.ErrorIs(t, err, providers.ErrMissing)
}

func TestProvider_DeleteMany(t *testing.T) {
	provider, _ := setupProvider(t)
	ctx := t.Context()

	keys := []string{"key1", "key2", "key3"}
	value := []byte("test-value")

	for _, key := range keys {
		err := provider.Save(ctx, key, value, 0)
		require.NoError(t, err)
	}

	err := provider.DeleteMany(ctx, keys...)
	require.NoError(t, err)

	for _, key := range keys {
		_, err := provider.Get(ctx, key)
		require.ErrorIs(t, err, providers.ErrMissing)
	}
}

func TestProvider_SaveWithTTL(t *testing.T) {
	provider, mr := setupProvider(t)
	ctx := t.Context()

	key := "test-key"
	value := []byte("test-value")
	ttl := 10 * time.Second

	err := provider.Save(ctx, key, value, ttl)
	require.NoError(t, err)

	// Verify key exists before TTL expires
	got, err := provider.Get(ctx, key)
	require.NoError(t, err)
	require.Equal(t, string(value), string(got))

	// Fast-forward time in miniredis
	mr.FastForward(15 * time.Second)

	// Verify key is expired
	_, err = provider.Get(ctx, key)
	require.ErrorIs(t, err, providers.ErrMissing)
}

func TestProvider_WithPrefix_Isolation(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	client := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	provider1 := redis.New(client, redis.WithPrefix("prefix1:"))
	provider2 := redis.New(client, redis.WithPrefix("prefix2:"))

	ctx := t.Context()
	key := "shared-key"
	value1 := []byte("value-1")
	value2 := []byte("value-2")

	// Save to provider1
	err := provider1.Save(ctx, key, value1, 0)
	require.NoError(t, err)

	// Save to provider2 with same key
	err = provider2.Save(ctx, key, value2, 0)
	require.NoError(t, err)

	// Verify provider1 still has its value
	got1, err := provider1.Get(ctx, key)
	require.NoError(t, err)
	require.Equal(t, string(value1), string(got1))

	// Verify provider2 has its value
	got2, err := provider2.Get(ctx, key)
	require.NoError(t, err)
	require.Equal(t, string(value2), string(got2))

	// Verify values are different
	require.NotEqual(t, string(got1), string(got2), "provider1 and provider2 values should be different")
}
