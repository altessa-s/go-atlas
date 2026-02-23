// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slices_test

import (
	"container/list"
	"slices"
	"testing"

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
	if !slices.Equal(got, want) {
		t.Errorf("List() = %v, want %v", got, want)
	}
}

func TestList_Empty(t *testing.T) {
	l := list.New()
	count := 0
	for range coreslices.List[int](l) {
		count++
	}
	if count != 0 {
		t.Errorf("List(empty) yielded %d items", count)
	}
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
	if !slices.Equal(got, want) {
		t.Errorf("Map() = %v, want %v", got, want)
	}
}

func TestChunk(t *testing.T) {
	input := []int{1, 2, 3, 4, 5}
	var got [][]int
	for chunk := range coreslices.Chunk(input, 2) {
		got = append(got, chunk)
	}
	if len(got) != 3 {
		t.Fatalf("Chunk() yielded %d chunks, want 3", len(got))
	}
	if !slices.Equal(got[0], []int{1, 2}) {
		t.Errorf("chunk[0] = %v", got[0])
	}
	if !slices.Equal(got[2], []int{5}) {
		t.Errorf("chunk[2] = %v", got[2])
	}
}

func TestValues(t *testing.T) {
	input := []string{"a", "b"}
	var got []string
	for v := range coreslices.Values(input) {
		got = append(got, v)
	}
	if !slices.Equal(got, input) {
		t.Errorf("Values() = %v, want %v", got, input)
	}
}

func TestBackward(t *testing.T) {
	input := []string{"a", "b", "c"}
	var got []string
	for _, v := range coreslices.Backward(input) {
		got = append(got, v)
	}
	want := []string{"c", "b", "a"}
	if !slices.Equal(got, want) {
		t.Errorf("Backward() = %v, want %v", got, want)
	}
}

func TestFilterSeq(t *testing.T) {
	input := []int{1, 2, 3, 4, 5}
	seq := coreslices.Values(input)
	var got []int
	for v := range coreslices.FilterSeq(seq, func(i int) bool { return i%2 == 0 }) {
		got = append(got, v)
	}
	want := []int{2, 4}
	if !slices.Equal(got, want) {
		t.Errorf("FilterSeq() = %v, want %v", got, want)
	}
}

func TestMapSeq(t *testing.T) {
	input := []int{1, 2, 3}
	seq := coreslices.Values(input)
	var got []int
	for v := range coreslices.MapSeq(seq, func(i int) int { return i * 10 }) {
		got = append(got, v)
	}
	want := []int{10, 20, 30}
	if !slices.Equal(got, want) {
		t.Errorf("MapSeq() = %v, want %v", got, want)
	}
}

func TestTake(t *testing.T) {
	input := []int{1, 2, 3, 4, 5}
	seq := coreslices.Values(input)
	var got []int
	for v := range coreslices.Take(seq, 3) {
		got = append(got, v)
	}
	want := []int{1, 2, 3}
	if !slices.Equal(got, want) {
		t.Errorf("Take() = %v, want %v", got, want)
	}
}

func TestPool(t *testing.T) {
	pool := coreslices.NewPool[int](10)
	s := pool.Get()
	if s == nil {
		t.Fatal("Get() returned nil")
	}
	*s = append(*s, 1, 2, 3)
	pool.Put(s)

	s2 := pool.GetWithCapacity(50)
	if cap(*s2) < 50 {
		t.Errorf("GetWithCapacity(50) cap = %d", cap(*s2))
	}
	pool.Put(s2)
}

func TestEnsureCapacity(t *testing.T) {
	s := make([]int, 0, 5)
	grew := coreslices.EnsureCapacity(&s, 100)
	if !grew {
		t.Error("EnsureCapacity should return true when growing")
	}
	if cap(s) < 100 {
		t.Errorf("cap = %d after EnsureCapacity(100)", cap(s))
	}

	grew = coreslices.EnsureCapacity(&s, 10)
	if grew {
		t.Error("EnsureCapacity should return false when no growth needed")
	}
}

func TestFilterLast_NotFound(t *testing.T) {
	input := []int{1, 2, 3}
	_, found := coreslices.FilterLast(input, func(i int) bool { return i > 10 })
	if found {
		t.Error("FilterLast should return false when no match")
	}
}
