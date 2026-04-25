// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slices_test

import (
	"container/list"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

func TestList(t *testing.T) {
	l := list.New()
	l.PushBack(1)
	l.PushBack(2)
	l.PushBack(3)

	var got []int
	for v := range coreslices.List[int](l) {
		got = append(got, v)
	}
	want := []int{1, 2, 3}
	require.True(t, slices.Equal(got, want), "List() = %v, want %v", got, want)
}

func TestList_Empty(t *testing.T) {
	l := list.New()
	count := 0
	for range coreslices.List[int](l) {
		count++
	}
	require.Equal(t, 0, count, "List(empty) yielded %d items", count)
}

func TestMap(t *testing.T) {
	input := []int{1, 2, 3}
	var got []string
	for v := range coreslices.Map(input, func(i int) string {
		return string(rune('a' + i - 1))
	}) {
		got = append(got, v)
	}
	want := []string{"a", "b", "c"}
	require.True(t, slices.Equal(got, want), "Map() = %v, want %v", got, want)
}

func TestChunk(t *testing.T) {
	input := []int{1, 2, 3, 4, 5}
	var got [][]int
	for chunk := range coreslices.Chunk(input, 2) {
		got = append(got, chunk)
	}
	require.Len(t, got, 3, "Chunk() yielded %d chunks, want 3", len(got))
	require.True(t, slices.Equal(got[0], []int{1, 2}), "chunk[0] = %v", got[0])
	require.True(t, slices.Equal(got[2], []int{5}), "chunk[2] = %v", got[2])
}

func TestValues(t *testing.T) {
	input := []string{"a", "b"}
	var got []string
	for v := range coreslices.Values(input) {
		got = append(got, v)
	}
	require.True(t, slices.Equal(got, input), "Values() = %v, want %v", got, input)
}

func TestBackward(t *testing.T) {
	input := []string{"a", "b", "c"}
	var got []string
	for _, v := range coreslices.Backward(input) {
		got = append(got, v)
	}
	want := []string{"c", "b", "a"}
	require.True(t, slices.Equal(got, want), "Backward() = %v, want %v", got, want)
}

func TestFilterSeq(t *testing.T) {
	input := []int{1, 2, 3, 4, 5}
	seq := coreslices.Values(input)
	var got []int
	for v := range coreslices.FilterSeq(seq, func(i int) bool { return i%2 == 0 }) {
		got = append(got, v)
	}
	want := []int{2, 4}
	require.True(t, slices.Equal(got, want), "FilterSeq() = %v, want %v", got, want)
}

func TestMapSeq(t *testing.T) {
	input := []int{1, 2, 3}
	seq := coreslices.Values(input)
	var got []int
	for v := range coreslices.MapSeq(seq, func(i int) int { return i * 10 }) {
		got = append(got, v)
	}
	want := []int{10, 20, 30}
	require.True(t, slices.Equal(got, want), "MapSeq() = %v, want %v", got, want)
}

func TestTake(t *testing.T) {
	input := []int{1, 2, 3, 4, 5}
	seq := coreslices.Values(input)
	var got []int
	for v := range coreslices.Take(seq, 3) {
		got = append(got, v)
	}
	want := []int{1, 2, 3}
	require.True(t, slices.Equal(got, want), "Take() = %v, want %v", got, want)
}

func TestPool(t *testing.T) {
	pool := coreslices.NewPool[int](10)
	s := pool.Get()
	require.NotNil(t, s)
	*s = append(*s, 1, 2, 3)
	pool.Put(s)

	s2 := pool.GetWithCapacity(50)
	require.GreaterOrEqual(t, cap(*s2), 50, "GetWithCapacity(50) cap = %d", cap(*s2))
	pool.Put(s2)
}

func TestEnsureCapacity(t *testing.T) {
	s := make([]int, 0, 5)
	grew := coreslices.EnsureCapacity(&s, 100)
	require.True(t, grew, "EnsureCapacity should return true when growing")
	require.GreaterOrEqual(t, cap(s), 100, "cap = %d after EnsureCapacity(100)", cap(s))

	grew = coreslices.EnsureCapacity(&s, 10)
	require.False(t, grew, "EnsureCapacity should return false when no growth needed")
}

func TestFilterLast_NotFound(t *testing.T) {
	input := []int{1, 2, 3}
	_, found := coreslices.FilterLast(input, func(i int) bool { return i > 10 })
	require.False(t, found, "FilterLast should return false when no match")
}
