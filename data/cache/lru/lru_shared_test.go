// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShardedCache_PutGet(t *testing.T) {
	c, err := NewShardedCache[string, int](100)
	require.NoError(t, err)
	c.Put("a", 1)
	v, ok := c.Get("a")
	require.True(t, ok)
	require.Equal(t, 1, v)
}

func TestShardedCache_Remove(t *testing.T) {
	c, _ := NewShardedCache[string, int](100)
	c.Put("a", 1)
	require.True(t, c.Remove("a"), "Remove returned false")
	_, ok := c.Get("a")
	require.False(t, ok, "key still present after remove")
}

func TestShardedCache_Len(t *testing.T) {
	c, _ := NewShardedCache[string, int](100)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)
	require.Equal(t, 3, c.Len())
}

func TestShardedCache_Purge(t *testing.T) {
	c, _ := NewShardedCache[string, int](100)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Purge()
	require.Equal(t, 0, c.Len())
}

func TestShardedCache_Concurrent(t *testing.T) {
	c, _ := NewShardedCache[int, int](1000)
	var wg sync.WaitGroup
	n := 100

	for i := range n {
		wg.Go(func() {
			c.Put(i, i*10)
			c.Get(i)
			c.Has(i)
		})
	}
	wg.Wait()

	require.NotEqual(t, 0, c.Len(), "expected items in cache after concurrent puts")
}
