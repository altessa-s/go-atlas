// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package maps_test

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/require"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

func TestKeys(t *testing.T) {
	t.Parallel()

	m := map[string]int{"a": 1, "b": 2}
	got := make(map[string]bool)
	for k := range coremaps.Keys(m) {
		got[k] = true
	}
	require.True(t, got["a"], "Keys() missing key 'a', got %v", got)
	require.True(t, got["b"], "Keys() missing key 'b', got %v", got)
}

func TestValues(t *testing.T) {
	t.Parallel()

	m := map[string]int{"a": 1, "b": 2}
	sum := 0
	for v := range coremaps.Values(m) {
		sum += v
	}
	require.Equal(t, 3, sum, "Values() sum = %d, want 3", sum)
}

func TestFilter(t *testing.T) {
	t.Parallel()

	m := map[string]int{"a": 1, "b": 2, "c": 3}
	got := maps.Collect(coremaps.Filter(m, func(_ string, v int) bool { return v > 1 }))
	require.Len(t, got, 2)
	_, ok := got["a"]
	require.False(t, ok, "Filter() should not include a=1")
}

func TestMapIter(t *testing.T) {
	t.Parallel()

	m := map[string]int{"a": 1}
	got := maps.Collect(coremaps.Map(m, func(k string, _ int) (string, string) {
		return k + "!", "val"
	}))
	require.Equal(t, "val", got["a!"], "Map() = %v", got)
}

func TestPool(t *testing.T) {
	t.Parallel()

	pool := coremaps.NewPool[string, int](10)
	m := pool.Get()
	require.NotNil(t, m)
	(*m)["key"] = 42
	pool.Put(m)

	m2 := pool.GetWithCapacity(50)
	require.NotNil(t, m2)
	pool.Put(m2)
}

// TestPool_GetWithCapacity_ReusesWithinDefault is a regression test for a
// bug where GetWithCapacity always reallocated: it compared expectedCapacity
// against len(*m), which is zero after Get's clear(), so every call threw
// away the pooled map and returned a fresh allocation. The fix fast-paths
// requests within defaultCap by returning the pooled pointer untouched.
func TestPool_GetWithCapacity_ReusesWithinDefault(t *testing.T) {
	t.Parallel()

	pool := coremaps.NewPool[string, int](64)

	// Seed the pool with a known map pointer.
	seeded := pool.Get()
	(*seeded)["seed"] = 1
	pool.Put(seeded)

	// A request within defaultCap must reuse the pooled allocation.
	reused := pool.GetWithCapacity(32)
	require.Equal(t, seeded, reused, "GetWithCapacity(<=defaultCap) allocated a new map; want reuse of pooled pointer %p, got %p", seeded, reused)
	require.Empty(t, *reused, "reused map not cleared: len=%d", len(*reused))
	pool.Put(reused)

	// A request exceeding defaultCap legitimately discards the pooled
	// map and allocates a new one — Go cannot expose allocated capacity
	// at runtime, so the pooled map might not satisfy the request.
	oversize := pool.GetWithCapacity(256)
	require.NotNil(t, oversize)
	pool.Put(oversize)
}

func TestWeakRef(t *testing.T) {
	t.Parallel()

	val := 42
	ref := coremaps.MakeWeakRef(&val)
	require.True(t, ref.IsAlive(), "IsAlive() should be true for live reference")
	got := ref.Value()
	require.NotNil(t, got, "Value() should not be nil")
	require.Equal(t, 42, *got)
}

func TestWeakMap_Basic(t *testing.T) {
	t.Parallel()

	wm := coremaps.NewWeakMap[string, int]()

	val := 42
	wm.Set("key", &val)

	got, ok := wm.Get("key")
	require.True(t, ok, "Get(key) should return true")
	require.NotNil(t, got)
	require.Equal(t, 42, *got)

	require.Equal(t, 1, wm.Len())

	wm.Delete("key")
	_, ok = wm.Get("key")
	require.False(t, ok, "Get(key) should return false after Delete")
}

func TestWeakMap_Range(t *testing.T) {
	t.Parallel()

	wm := coremaps.NewWeakMap[string, int]()
	v1 := 1
	v2 := 2
	wm.Set("a", &v1)
	wm.Set("b", &v2)

	count := 0
	wm.Range(func(key string, value *int) bool {
		count++
		return true
	})
	require.Equal(t, 2, count)
}

func TestWeakMap_Cleanup(t *testing.T) {
	t.Parallel()

	wm := coremaps.NewWeakMap[string, int]()
	v := 42
	wm.Set("key", &v)
	wm.Cleanup() // should not panic
}
