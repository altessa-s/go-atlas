// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cuckoo_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
)

func TestNew(t *testing.T) {
	s := memory.New(memory.WithCapacity(1000))
	f := cuckoo.New(s)
	require.NotNil(t, f, "New() returned nil")
}

func TestFilter_AddAndMightExist(t *testing.T) {
	s := memory.New(memory.WithCapacity(1000))
	f := cuckoo.New(s)
	ctx := t.Context()

	require.NoError(t, f.Add(ctx, "hello"))

	exists, err := f.MightExist(ctx, "hello")
	require.NoError(t, err)
	require.True(t, exists, "MightExist() should return true for added item")
}

func TestFilter_MightExist_NotAdded(t *testing.T) {
	s := memory.New(memory.WithCapacity(1000))
	f := cuckoo.New(s)
	ctx := t.Context()

	exists, err := f.MightExist(ctx, "notadded")
	require.NoError(t, err)
	require.False(t, exists, "MightExist() should return false for non-existent item")
}

func TestFilter_AddBatch(t *testing.T) {
	s := memory.New(memory.WithCapacity(1000))
	f := cuckoo.New(s)
	ctx := t.Context()

	values := func(yield func(string) bool) {
		for _, v := range []string{"a", "b", "c"} {
			if !yield(v) {
				return
			}
		}
	}

	require.NoError(t, f.AddBatch(ctx, values))

	for _, v := range []string{"a", "b", "c"} {
		exists, _ := f.MightExist(ctx, v)
		require.True(t, exists, "MightExist(%q) = false, want true", v)
	}
}

func TestFilter_Delete(t *testing.T) {
	s := memory.New(memory.WithCapacity(1000))
	f := cuckoo.New(s)
	ctx := t.Context()

	_ = f.Add(ctx, "item")
	deleted, err := f.Delete(ctx, "item")
	require.NoError(t, err)
	require.True(t, deleted, "Delete() should return true for existing item")

	exists, _ := f.MightExist(ctx, "item")
	require.False(t, exists, "MightExist() should return false after Delete()")
}

func TestFilter_Delete_NotExist(t *testing.T) {
	s := memory.New(memory.WithCapacity(1000))
	f := cuckoo.New(s)
	ctx := t.Context()

	deleted, err := f.Delete(ctx, "nope")
	require.NoError(t, err)
	require.False(t, deleted, "Delete() should return false for non-existent item")
}

func TestFilter_Stats(t *testing.T) {
	s := memory.New(memory.WithCapacity(1000))
	f := cuckoo.New(s)
	ctx := t.Context()

	_ = f.Add(ctx, "x")
	stats, err := f.Stats(ctx)
	require.NoError(t, err)
	require.Equal(t, "memory", stats.StorageType)
}

func TestFilter_Close(t *testing.T) {
	s := memory.New(memory.WithCapacity(1000))
	f := cuckoo.New(s)
	require.NoError(t, f.Close(t.Context()))
}
