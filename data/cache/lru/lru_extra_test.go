// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewCache_InvalidSize(t *testing.T) {
	_, err := NewCache[string, int](0)
	require.Error(t, err, "NewCache(0) should return error")
}

func TestNewShardedCache_SmallSize(t *testing.T) {
	// Size 0 still works (shardSize becomes 1)
	c, err := NewShardedCache[string, int](0)
	require.NoError(t, err)
	c.Put("a", 1)
	v, ok := c.Get("a")
	require.True(t, ok)
	require.Equal(t, 1, v)
}

func TestNewShardedCache_WithShardCount(t *testing.T) {
	c, err := NewShardedCache[string, int](100, WithShardCount(32))
	require.NoError(t, err)
	c.Put("a", 1)
	v, ok := c.Get("a")
	require.True(t, ok)
	require.Equal(t, 1, v)
}

func TestShardedCache_Keys(t *testing.T) {
	c, _ := NewShardedCache[string, int](100)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)

	keys := make(map[string]bool)
	for k := range c.Keys() {
		keys[k] = true
	}
	require.Len(t, keys, 3)
}

func TestShardedCache_All(t *testing.T) {
	c, _ := NewShardedCache[string, int](100)
	c.Put("x", 10)
	c.Put("y", 20)

	pairs := make(map[string]int)
	for k, v := range c.All() {
		pairs[k] = v
	}
	require.Equal(t, 10, pairs["x"])
	require.Equal(t, 20, pairs["y"])
}

func TestShardedCache_GetOrCompute_Miss(t *testing.T) {
	c, _ := NewShardedCache[string, string](100)
	ctx := t.Context()

	v, err := c.GetOrCompute(ctx, "key", func(ctx context.Context) (string, error) {
		return "computed", nil
	})
	require.NoError(t, err)
	require.Equal(t, "computed", v)

	v2, ok := c.Get("key")
	require.True(t, ok, "value not cached after GetOrCompute")
	require.Equal(t, "computed", v2)
}

func TestShardedCache_GetOrCompute_Hit(t *testing.T) {
	c, _ := NewShardedCache[string, string](100)
	c.Put("key", "existing")
	ctx := t.Context()

	v, err := c.GetOrCompute(ctx, "key", func(ctx context.Context) (string, error) {
		t.Error("compute should not be called on hit")
		return "", nil
	})
	require.NoError(t, err)
	require.Equal(t, "existing", v)
}

func TestShardedCache_GetOrCompute_Error(t *testing.T) {
	c, _ := NewShardedCache[string, string](100)
	ctx := t.Context()

	wantErr := errors.New("fail")
	_, err := c.GetOrCompute(ctx, "key", func(ctx context.Context) (string, error) {
		return "", wantErr
	})
	require.ErrorIs(t, err, wantErr)
}

func TestShardedCache_Has(t *testing.T) {
	c, _ := NewShardedCache[string, int](100)
	c.Put("a", 1)
	require.True(t, c.Has("a"), "Has returned false for existing key")
	require.False(t, c.Has("b"), "Has returned true for missing key")
}

func TestShardedCache_IntKey(t *testing.T) {
	c, _ := NewShardedCache[int, string](100)
	c.Put(42, "hello")
	v, ok := c.Get(42)
	require.True(t, ok)
	require.Equal(t, "hello", v)
}

func TestShardedCache_Int64Key(t *testing.T) {
	c, _ := NewShardedCache[int64, string](100)
	c.Put(int64(99), "world")
	v, ok := c.Get(int64(99))
	require.True(t, ok)
	require.Equal(t, "world", v)
}

func TestShardedCache_Uint64Key(t *testing.T) {
	c, _ := NewShardedCache[uint64, string](100)
	c.Put(uint64(77), "test")
	v, ok := c.Get(uint64(77))
	require.True(t, ok)
	require.Equal(t, "test", v)
}
