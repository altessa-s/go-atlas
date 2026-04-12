// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"

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
