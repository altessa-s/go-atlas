// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	memory "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
)

func TestStorage_AddAndMightExist(t *testing.T) {
	storage := memory.New(memory.WithCapacity(1000))
	ctx := t.Context()

	err := storage.Add(ctx, "hello")
	require.NoError(t, err)

	exists, err := storage.MightExist(ctx, "hello")
	require.NoError(t, err)
	require.True(t, exists, "Expected 'hello' to exist in filter")
}

func TestStorage_MightExist_NotAdded(t *testing.T) {
	storage := memory.New(memory.WithCapacity(1000))
	ctx := t.Context()

	exists, err := storage.MightExist(ctx, "notadded")
	require.NoError(t, err)
	require.False(t, exists, "Expected 'notadded' to not exist in filter")
}

func TestStorage_AddBatch(t *testing.T) {
	storage := memory.New(memory.WithCapacity(1000))
	ctx := t.Context()

	values := func(yield func(string) bool) {
		items := []string{"item1", "item2", "item3"}
		for _, item := range items {
			if !yield(item) {
				return
			}
		}
	}

	err := storage.AddBatch(ctx, values)
	require.NoError(t, err)

	for _, item := range []string{"item1", "item2", "item3"} {
		exists, err := storage.MightExist(ctx, item)
		require.NoError(t, err)
		require.True(t, exists, "Expected '%s' to exist in filter", item)
	}
}

func TestStorage_Delete(t *testing.T) {
	storage := memory.New(memory.WithCapacity(1000))
	ctx := t.Context()

	err := storage.Add(ctx, "item")
	require.NoError(t, err)

	deleted, err := storage.Delete(ctx, "item")
	require.NoError(t, err)
	require.True(t, deleted, "Expected Delete to return true for existing item")

	exists, err := storage.MightExist(ctx, "item")
	require.NoError(t, err)
	require.False(t, exists, "Expected 'item' to not exist after deletion")
}

func TestStorage_Delete_NotExist(t *testing.T) {
	storage := memory.New(memory.WithCapacity(1000))
	ctx := t.Context()

	deleted, err := storage.Delete(ctx, "nonexistent")
	require.NoError(t, err)
	require.False(t, deleted, "Expected Delete to return false for non-existent item")
}

func TestStorage_Stats(t *testing.T) {
	storage := memory.New(memory.WithCapacity(1000))
	ctx := t.Context()

	for i := range 10 {
		err := storage.Add(ctx, string(rune('a'+i)))
		require.NoError(t, err)
	}

	stats, err := storage.Stats(ctx)
	require.NoError(t, err)

	require.True(t, stats.Capacity > 0, "Expected capacity > 0, got %d", stats.Capacity)

	require.Equal(t, "memory", stats.StorageType)
}

func TestStorage_Close(t *testing.T) {
	storage := memory.New(memory.WithCapacity(1000))
	ctx := t.Context()

	err := storage.Close(ctx)
	require.NoError(t, err)
}
