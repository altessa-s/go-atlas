// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package maps_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

func TestNewImmutableMap_Nil(t *testing.T) {
	t.Parallel()

	m := coremaps.NewImmutableMap[string, int](nil)

	require.Equal(t, 0, m.Len())

	v, ok := m.Get("any")
	require.False(t, ok)
	require.Zero(t, v)
}

func TestNewImmutableMap_Empty(t *testing.T) {
	t.Parallel()

	m := coremaps.NewImmutableMap(map[string]int{})

	require.Equal(t, 0, m.Len())
	require.False(t, m.Contains("any"))
}

func TestNewImmutableMap_SingleEntry(t *testing.T) {
	t.Parallel()

	m := coremaps.NewImmutableMap(map[string]int{"hello": 42})

	require.Equal(t, 1, m.Len())

	v, ok := m.Get("hello")
	require.True(t, ok)
	require.Equal(t, 42, v)

	require.True(t, m.Contains("hello"))
	require.False(t, m.Contains("world"))
}

func TestNewImmutableMap_Basic(t *testing.T) {
	t.Parallel()

	src := map[string]int{
		"a": 1,
		"b": 2,
		"c": 3,
		"d": 4,
		"e": 5,
	}
	m := coremaps.NewImmutableMap(src)

	require.Equal(t, len(src), m.Len())

	for k, v := range src {
		got, ok := m.Get(k)
		require.True(t, ok, "key %q not found", k)
		require.Equal(t, v, got, "key %q", k)
	}

	_, ok := m.Get("missing")
	require.False(t, ok)
}

func TestNewImmutableMap_Large(t *testing.T) {
	t.Parallel()

	const n = 100_000
	src := make(map[int]int, n)
	for i := range n {
		src[i] = i * 7
	}

	m := coremaps.NewImmutableMap(src)
	require.Equal(t, n, m.Len())

	for i := range n {
		v, ok := m.Get(i)
		require.True(t, ok, "key %d not found", i)
		require.Equal(t, i*7, v, "key %d", i)
	}

	_, ok := m.Get(n)
	require.False(t, ok)
}

func TestNewImmutableMapFromEntries_Basic(t *testing.T) {
	t.Parallel()

	keys := []string{"a", "b", "c"}
	vals := []int{1, 2, 3}

	m := coremaps.NewImmutableMapFromEntries(keys, vals)
	require.Equal(t, 3, m.Len())

	for i, k := range keys {
		v, ok := m.Get(k)
		require.True(t, ok)
		require.Equal(t, vals[i], v)
	}
}

func TestNewImmutableMapFromEntries_DuplicateKeys(t *testing.T) {
	t.Parallel()

	keys := []string{"a", "b", "a"}
	vals := []int{1, 2, 3}

	m := coremaps.NewImmutableMapFromEntries(keys, vals)
	require.Equal(t, 2, m.Len())

	v, ok := m.Get("a")
	require.True(t, ok)
	require.Equal(t, 3, v) // last-write-wins
}

func TestNewImmutableMapFromEntries_LengthMismatch(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() {
		coremaps.NewImmutableMapFromEntries([]string{"a"}, []int{1, 2})
	})
}

func TestNewImmutableMapFromEntries_Empty(t *testing.T) {
	t.Parallel()

	m := coremaps.NewImmutableMapFromEntries[string, int](nil, nil)
	require.Equal(t, 0, m.Len())
}

func TestImmutableMap_IntKeys(t *testing.T) {
	t.Parallel()

	src := map[uint64]uint64{
		1:    100,
		2:    200,
		1000: 999,
	}
	m := coremaps.NewImmutableMap(src)

	for k, v := range src {
		got, ok := m.Get(k)
		require.True(t, ok)
		require.Equal(t, v, got)
	}
}

type testStructKey struct {
	A int
	B int
}

func TestImmutableMap_StructKeys(t *testing.T) {
	t.Parallel()

	src := map[testStructKey]string{
		{1, 2}: "one-two",
		{3, 4}: "three-four",
	}
	m := coremaps.NewImmutableMap(src)

	for k, v := range src {
		got, ok := m.Get(k)
		require.True(t, ok)
		require.Equal(t, v, got)
	}

	_, ok := m.Get(testStructKey{5, 6})
	require.False(t, ok)
}

func TestImmutableMap_All(t *testing.T) {
	t.Parallel()

	src := map[string]int{"a": 1, "b": 2, "c": 3}
	m := coremaps.NewImmutableMap(src)

	got := make(map[string]int)
	for k, v := range m.All() {
		got[k] = v
	}
	require.Equal(t, src, got)
}

func TestImmutableMap_All_EarlyBreak(t *testing.T) {
	t.Parallel()

	src := map[string]int{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}
	m := coremaps.NewImmutableMap(src)

	count := 0
	for range m.All() {
		count++
		if count == 2 {
			break
		}
	}
	require.Equal(t, 2, count)
}

func TestImmutableMap_Keys(t *testing.T) {
	t.Parallel()

	src := map[string]int{"a": 1, "b": 2, "c": 3}
	m := coremaps.NewImmutableMap(src)

	got := make(map[string]bool)
	for k := range m.Keys() {
		got[k] = true
	}
	for k := range src {
		require.True(t, got[k], "missing key %q", k)
	}
	require.Len(t, got, len(src))
}

func TestImmutableMap_Values(t *testing.T) {
	t.Parallel()

	src := map[string]int{"a": 1, "b": 2, "c": 3}
	m := coremaps.NewImmutableMap(src)

	var got []int
	for v := range m.Values() {
		got = append(got, v)
	}
	require.Len(t, got, len(src))
	require.ElementsMatch(t, []int{1, 2, 3}, got)
}

func TestImmutableMap_ManyCollisions(t *testing.T) {
	t.Parallel()

	// Fill a small table densely to force many H2 collisions and multi-group
	// probing. With groupSize=8 and 87.5% load, 14 entries → 16 slots (2 groups).
	const n = 14
	src := make(map[int]int, n)
	for i := range n {
		src[i] = i * 3
	}

	m := coremaps.NewImmutableMap(src)
	require.Equal(t, n, m.Len())

	for k, v := range src {
		got, ok := m.Get(k)
		require.True(t, ok, "key %d not found", k)
		require.Equal(t, v, got, "key %d", k)
	}
}

func TestImmutableMap_ZeroValue(t *testing.T) {
	t.Parallel()

	var m coremaps.ImmutableMap[string, int]

	require.Equal(t, 0, m.Len())
	require.False(t, m.Contains("x"))

	v, ok := m.Get("x")
	require.False(t, ok)
	require.Zero(t, v)

	// Iterators on zero-value must not panic.
	for range m.All() {
		t.Fatal("unexpected entry")
	}
	for range m.Keys() {
		t.Fatal("unexpected key")
	}
	for range m.Values() {
		t.Fatal("unexpected value")
	}
}

func TestImmutableMap_Keys_EarlyBreak(t *testing.T) {
	t.Parallel()

	src := map[string]int{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}
	m := coremaps.NewImmutableMap(src)

	count := 0
	for range m.Keys() {
		count++
		if count == 2 {
			break
		}
	}
	require.Equal(t, 2, count)
}

func TestImmutableMap_Values_EarlyBreak(t *testing.T) {
	t.Parallel()

	src := map[string]int{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}
	m := coremaps.NewImmutableMap(src)

	count := 0
	for range m.Values() {
		count++
		if count == 2 {
			break
		}
	}
	require.Equal(t, 2, count)
}

func TestImmutableMap_ConcurrentGet(t *testing.T) {
	t.Parallel()

	const n = 10_000
	src := make(map[int]int, n)
	for i := range n {
		src[i] = i
	}
	m := coremaps.NewImmutableMap(src)

	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			start := g * (n / 8)
			end := start + (n / 8)
			for i := start; i < end; i++ {
				v, ok := m.Get(i)
				if !ok || v != i {
					t.Errorf("Get(%d) = %d, %v; want %d, true", i, v, ok, i)
					return
				}
			}
		})
	}
	wg.Wait()
}

func TestImmutableMap_ExactlySevenOfEight(t *testing.T) {
	t.Parallel()

	// 7 entries → 1 group of 8 slots at exactly 87.5% load.
	src := map[int]int{0: 0, 1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 6: 6}
	m := coremaps.NewImmutableMap(src)
	require.Equal(t, 7, m.Len())

	for k, v := range src {
		got, ok := m.Get(k)
		require.True(t, ok, "key %d", k)
		require.Equal(t, v, got)
	}
	require.False(t, m.Contains(7))
}

func TestImmutableMap_GrowingSizes(t *testing.T) {
	t.Parallel()

	sizes := []int{1, 7, 8, 9, 15, 16, 17, 63, 64, 65, 255, 256, 1000}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("Size_%d", size), func(t *testing.T) {
			t.Parallel()

			src := make(map[int]int, size)
			for i := range size {
				src[i] = i
			}

			m := coremaps.NewImmutableMap(src)
			require.Equal(t, size, m.Len())

			for k, v := range src {
				got, ok := m.Get(k)
				require.True(t, ok, "key %d not found", k)
				require.Equal(t, v, got)
			}
		})
	}
}
