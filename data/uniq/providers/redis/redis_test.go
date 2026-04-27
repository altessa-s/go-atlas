// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	uniqredis "github.com/altessa-s/go-atlas/data/uniq/providers/redis"
	goredis "github.com/redis/go-redis/v9"
)

func setupProvider(tb testing.TB) *uniqredis.Provider {
	tb.Helper()
	mr := miniredis.RunT(tb)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	tb.Cleanup(func() { client.Close() })
	return uniqredis.New(client)
}

func TestNew(t *testing.T) {
	p := setupProvider(t)
	require.NotNil(t, p, "New() returned nil")
}

func TestProvider_Add_Exist(t *testing.T) {
	p := setupProvider(t)
	ctx := t.Context()

	require.NoError(t, p.Add(ctx, "key1"))

	exists, err := p.Exist(ctx, "key1")
	require.NoError(t, err)
	require.True(t, exists, "Exist() should return true for added key")
}

func TestProvider_Exist_NotFound(t *testing.T) {
	p := setupProvider(t)
	exists, err := p.Exist(t.Context(), "missing")
	require.NoError(t, err)
	require.False(t, exists, "Exist() should return false for missing key")
}

func TestProvider_AddWithValue_GetValue(t *testing.T) {
	p := setupProvider(t)
	ctx := t.Context()

	require.NoError(t, p.AddWithValue(ctx, "key1", []byte("hello")))

	val, err := p.GetValue(ctx, "key1")
	require.NoError(t, err)
	require.Equal(t, "hello", string(val))
}

func TestProvider_GetValue_NotFound(t *testing.T) {
	p := setupProvider(t)
	val, err := p.GetValue(t.Context(), "missing")
	require.NoError(t, err)
	require.Nil(t, val)
}

func TestProvider_Remove(t *testing.T) {
	p := setupProvider(t)
	ctx := t.Context()

	_ = p.Add(ctx, "key1")
	require.NoError(t, p.Remove(ctx, "key1"))

	exists, _ := p.Exist(ctx, "key1")
	require.False(t, exists, "Exist() should return false after Remove()")
}

func TestProvider_Clear(t *testing.T) {
	p := setupProvider(t)
	ctx := t.Context()

	_ = p.Add(ctx, "key1")
	_ = p.Add(ctx, "key2")

	require.NoError(t, p.Clear(ctx))

	e1, _ := p.Exist(ctx, "key1")
	e2, _ := p.Exist(ctx, "key2")
	require.False(t, e1, "key1 should not exist after Clear()")
	require.False(t, e2, "key2 should not exist after Clear()")
}

func TestProvider_Add_Overwrite(t *testing.T) {
	p := setupProvider(t)
	ctx := t.Context()

	_ = p.AddWithValue(ctx, "key1", []byte("v1"))
	_ = p.AddWithValue(ctx, "key1", []byte("v2"))

	val, _ := p.GetValue(ctx, "key1")
	require.Equal(t, "v2", string(val))
}

func TestProvider_WithCustomPrefix(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	p := uniqredis.New(client, uniqredis.WithPrefix("custom:"))
	ctx := t.Context()

	_ = p.Add(ctx, "key1")
	exists, _ := p.Exist(ctx, "key1")
	require.True(t, exists, "Exist() should return true with custom prefix")
}

func TestProvider_Probe_OK(t *testing.T) {
	p := setupProvider(t)
	require.NoError(t, p.Probe(t.Context()))
}

func TestProvider_Probe_AfterClientClose(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	p := uniqredis.New(client)

	require.NoError(t, client.Close())
	require.Error(t, p.Probe(t.Context()),
		"Probe() after closing the underlying Redis client should return an error")
}

func TestProvider_TryAdd_NewKey(t *testing.T) {
	p := setupProvider(t)
	ok, err := p.TryAdd(t.Context(), "fresh-key", 0)
	require.NoError(t, err)
	require.True(t, ok, "TryAdd() on a missing key must report acquired=true")
}

func TestProvider_TryAdd_ExistingKey(t *testing.T) {
	p := setupProvider(t)
	ctx := t.Context()

	require.NoError(t, p.Add(ctx, "taken"))

	ok, err := p.TryAdd(ctx, "taken", 0)
	require.NoError(t, err)
	require.False(t, ok, "TryAdd() on an existing key must report acquired=false (race closed)")
}

func TestProvider_TryAddWithValue_NewKey(t *testing.T) {
	p := setupProvider(t)
	ctx := t.Context()

	ok, err := p.TryAddWithValue(ctx, "k", []byte("v1"), 0)
	require.NoError(t, err)
	require.True(t, ok)

	val, err := p.GetValue(ctx, "k")
	require.NoError(t, err)
	require.Equal(t, "v1", string(val))
}

func TestProvider_TryAddWithValue_ExistingKey_DoesNotOverwrite(t *testing.T) {
	p := setupProvider(t)
	ctx := t.Context()

	require.NoError(t, p.AddWithValue(ctx, "k", []byte("original")))

	ok, err := p.TryAddWithValue(ctx, "k", []byte("attempted-overwrite"), 0)
	require.NoError(t, err)
	require.False(t, ok, "TryAddWithValue() on existing key must not acquire")

	val, err := p.GetValue(ctx, "k")
	require.NoError(t, err)
	require.Equal(t, "original", string(val), "TryAddWithValue() must not overwrite existing value")
}

func TestProvider_TryAdd_PerCallTtl_OverridesDefault(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	// Long default TTL — per-call short TTL must win.
	p := uniqredis.New(client, uniqredis.WithTtl(time.Hour))
	ctx := t.Context()

	ok, err := p.TryAdd(ctx, "ephemeral", 100*time.Millisecond)
	require.NoError(t, err)
	require.True(t, ok)

	mr.FastForward(200 * time.Millisecond)

	exists, err := p.Exist(ctx, "ephemeral")
	require.NoError(t, err)
	require.False(t, exists, "key should have expired under per-call TTL, not the long default")
}

func TestProvider_TryAdd_ZeroTtl_UsesProviderDefault(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	// Short default TTL — passing 0 must fall back to it.
	p := uniqredis.New(client, uniqredis.WithTtl(100*time.Millisecond))
	ctx := t.Context()

	ok, err := p.TryAdd(ctx, "k", 0)
	require.NoError(t, err)
	require.True(t, ok)

	mr.FastForward(200 * time.Millisecond)

	exists, err := p.Exist(ctx, "k")
	require.NoError(t, err)
	require.False(t, exists, "ttl=0 must apply provider default; key should have expired")
}

func TestProvider_TryAddWithValue_PerCallTtl_StoresValueAndExpires(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	p := uniqredis.New(client, uniqredis.WithTtl(time.Hour))
	ctx := t.Context()

	ok, err := p.TryAddWithValue(ctx, "k", []byte("payload"), 100*time.Millisecond)
	require.NoError(t, err)
	require.True(t, ok)

	val, err := p.GetValue(ctx, "k")
	require.NoError(t, err)
	require.Equal(t, "payload", string(val), "value must be readable while key is alive")

	mr.FastForward(200 * time.Millisecond)

	exists, err := p.Exist(ctx, "k")
	require.NoError(t, err)
	require.False(t, exists, "key must expire at per-call TTL")
}
