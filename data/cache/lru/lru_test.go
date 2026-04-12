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

func TestCache_PutGet(t *testing.T) {
	c, err := NewCache[string, int](10)
	require.NoError(t, err)
	c.Put("a", 1)
	v, ok := c.Get("a")
	require.True(t, ok)
	require.Equal(t, 1, v)
}

func TestCache_Remove(t *testing.T) {
	c, _ := NewCache[string, int](10)
	c.Put("a", 1)
	require.True(t, c.Remove("a"), "Remove returned false for existing key")
	_, ok := c.Get("a")
	require.False(t, ok, "key still present after remove")
}

func TestCache_Has(t *testing.T) {
	c, _ := NewCache[string, int](10)
	c.Put("a", 1)
	require.True(t, c.Has("a"), "Has returned false for existing key")
	require.False(t, c.Has("b"), "Has returned true for missing key")
}

func TestCache_Len(t *testing.T) {
	c, _ := NewCache[string, int](10)
	c.Put("a", 1)
	c.Put("b", 2)
	require.Equal(t, 2, c.Len())
}

func TestCache_Purge(t *testing.T) {
	c, _ := NewCache[string, int](10)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Purge()
	require.Equal(t, 0, c.Len())
}

func TestCache_Keys(t *testing.T) {
	c, _ := NewCache[string, int](10)
	c.Put("a", 1)
	c.Put("b", 2)

	keys := make(map[string]bool)
	for k := range c.Keys() {
		keys[k] = true
	}
	require.True(t, keys["a"], "missing key a")
	require.True(t, keys["b"], "missing key b")
}

func TestCache_All(t *testing.T) {
	c, _ := NewCache[string, int](10)
	c.Put("a", 1)
	c.Put("b", 2)

	pairs := make(map[string]int)
	for k, v := range c.All() {
		pairs[k] = v
	}
	require.Equal(t, 1, pairs["a"])
	require.Equal(t, 2, pairs["b"])
}

func TestCache_GetOrCompute_Miss(t *testing.T) {
	c, _ := NewCache[string, string](10)
	ctx := t.Context()

	v, err := c.GetOrCompute(ctx, "key", func(ctx context.Context) (string, error) {
		return "computed", nil
	})
	require.NoError(t, err)
	require.Equal(t, "computed", v)

	// Should now be cached.
	v2, ok := c.Get("key")
	require.True(t, ok, "value not cached after GetOrCompute")
	require.Equal(t, "computed", v2)
}

func TestCache_GetOrCompute_Hit(t *testing.T) {
	c, _ := NewCache[string, string](10)
	c.Put("key", "existing")
	ctx := t.Context()

	v, err := c.GetOrCompute(ctx, "key", func(ctx context.Context) (string, error) {
		t.Error("compute should not be called on hit")
		return "", nil
	})
	require.NoError(t, err)
	require.Equal(t, "existing", v)
}

func TestCache_Eviction(t *testing.T) {
	c, _ := NewCache[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3) // should evict "a"

	require.False(t, c.Has("a"), "expected 'a' to be evicted")
	require.True(t, c.Has("b"), "expected 'b' to still exist")
	require.True(t, c.Has("c"), "expected 'c' to still exist")
}

func TestCache_GetOrCompute_Error(t *testing.T) {
	c, _ := NewCache[string, string](10)
	ctx := t.Context()

	wantErr := errors.New("fail")
	_, err := c.GetOrCompute(ctx, "key", func(ctx context.Context) (string, error) {
		return "", wantErr
	})
	require.ErrorIs(t, err, wantErr)
}
