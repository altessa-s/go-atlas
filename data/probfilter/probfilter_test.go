// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilter_test

import (
	"context"
	"iter"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo"

	bloommemory "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/memory"
	cuckoomemory "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
)

func TestManager(t *testing.T) {
	mgr := probfilter.NewManager()

	bStorage := bloommemory.New(bloommemory.WithExpectedItems(100))
	bFilter := bloom.New(bStorage)

	cStorage := cuckoomemory.New(cuckoomemory.WithCapacity(100))
	cFilter := cuckoo.New(cStorage)

	require.NoError(t, mgr.Register("bloom", bFilter))
	require.NoError(t, mgr.Register("cuckoo", cFilter))

	// Test Names iterator
	var names []string
	for name := range mgr.Names() {
		names = append(names, name)
	}
	slices.Sort(names)
	expectedNames := []string{"bloom", "cuckoo"}
	require.True(t, slices.Equal(names, expectedNames), "expected names %v, got %v", expectedNames, names)

	// Test Filters iterator
	count := 0
	for name, f := range mgr.Filters() {
		require.NotEmpty(t, name, "empty name in iterator")
		require.NotNil(t, f, "nil filter in iterator")
		count++
	}
	require.Equal(t, 2, count)

	require.NoError(t, mgr.Close())
}

func TestBloomFilter_AddBatch(t *testing.T) {
	ctx := t.Context()
	storage := bloommemory.New(bloommemory.WithExpectedItems(1000))
	filter := bloom.New(storage)

	values := []string{"a", "b", "c"}
	iter := func(yield func(string) bool) {
		for _, v := range values {
			if !yield(v) {
				return
			}
		}
	}

	require.NoError(t, filter.AddBatch(ctx, iter))

	for _, v := range values {
		exists, err := filter.MightExist(ctx, v)
		require.NoError(t, err)
		require.True(t, exists, "expected value %s to exist", v)
	}
}

func TestCuckooFilter_AddBatch(t *testing.T) {
	ctx := t.Context()
	storage := cuckoomemory.New(cuckoomemory.WithCapacity(1000))
	filter := cuckoo.New(storage)

	values := []string{"x", "y", "z"}
	iter := func(yield func(string) bool) {
		for _, v := range values {
			if !yield(v) {
				return
			}
		}
	}

	require.NoError(t, filter.AddBatch(ctx, iter))

	for _, v := range values {
		exists, err := filter.MightExist(ctx, v)
		require.NoError(t, err)
		require.True(t, exists, "expected value %s to exist", v)
	}

	// Test Delete
	ok, err := filter.Delete(ctx, "x")
	require.NoError(t, err)
	require.True(t, ok, "expected delete to return true")

	exists, _ := filter.MightExist(ctx, "x")
	require.False(t, exists, "expected value x to not exist after delete")
}

type mockLoader struct {
	countCalled  int
	streamCalled int
	items        []string
}

func (m *mockLoader) Count(ctx context.Context) (int64, error) {
	m.countCalled++
	return int64(len(m.items)), nil
}

func (m *mockLoader) StreamValues(ctx context.Context) iter.Seq2[string, error] {
	m.streamCalled++
	return func(yield func(string, error) bool) {
		for _, v := range m.items {
			if !yield(v, nil) {
				return
			}
		}
	}
}

func TestBloomFilter_Rebuild_Optimization(t *testing.T) {
	ctx := t.Context()
	storage := bloommemory.New(bloommemory.WithExpectedItems(100))
	filter := bloom.New(storage)

	loader := &mockLoader{
		items: []string{"a", "b", "c"},
	}

	require.NoError(t, filter.Rebuild(ctx, loader))

	// Verify optimization: Count should be called once, StreamValues once (for loading)
	// If optimization wasn't working, StreamValues might be called twice (once for count, once for load)
	require.Equal(t, 1, loader.countCalled)
	require.Equal(t, 1, loader.streamCalled)

	// Verify items were loaded
	for _, v := range loader.items {
		exists, _ := filter.MightExist(ctx, v)
		require.True(t, exists, "expected value %s to exist after rebuild", v)
	}
}
