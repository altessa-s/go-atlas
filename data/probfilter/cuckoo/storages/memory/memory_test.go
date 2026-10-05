// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"context"
	"fmt"
	"slices"
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

func TestStorage_Stage_CommitReplacesContents(t *testing.T) {
	t.Parallel()
	storage := memory.New(memory.WithCapacity(64))
	ctx := t.Context()

	require.NoError(t, storage.Add(ctx, "old"))

	// More items than the configured capacity: the replacement is sized for them.
	values := make([]string, 500)
	for i := range values {
		values[i] = fmt.Sprintf("v-%d", i)
	}
	st, err := storage.Stage(ctx, int64(len(values)))
	require.NoError(t, err)
	require.NoError(t, st.AddBatch(ctx, slices.Values(values)))

	exists, err := storage.MightExist(ctx, "old")
	require.NoError(t, err)
	require.True(t, exists, "live contents must stay visible while staging")

	require.NoError(t, st.Commit(ctx))

	for _, v := range values {
		exists, err := storage.MightExist(ctx, v)
		require.NoError(t, err)
		require.True(t, exists, "MightExist(%q) after Commit", v)
	}
	stats, err := storage.Stats(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, stats.Capacity, int64(len(values)))
	require.Equal(t, int64(len(values)), stats.ItemCount)
}

func TestStorage_Stage_AbortKeepsContents(t *testing.T) {
	t.Parallel()
	storage := memory.New(memory.WithCapacity(64))
	ctx := t.Context()

	require.NoError(t, storage.Add(ctx, "old"))
	// Fingerprints are seeded per filter, so pick a probe the live filter
	// verifiably does not report before staging it.
	probe := absentProbe(t, storage)
	st, err := storage.Stage(ctx, 1)
	require.NoError(t, err)
	require.NoError(t, st.AddBatch(ctx, slices.Values([]string{probe})))
	require.NoError(t, st.Abort(ctx))

	exists, err := storage.MightExist(ctx, "old")
	require.NoError(t, err)
	require.True(t, exists)
	exists, err = storage.MightExist(ctx, probe)
	require.NoError(t, err)
	require.False(t, exists, "an aborted replacement must not change the live filter")
}

func TestStorage_Stage_FullReplacement(t *testing.T) {
	t.Parallel()
	storage := memory.New(memory.WithCapacity(4))
	ctx := t.Context()

	// Staged for 1 item, fed far more: the replacement fills up.
	st, err := storage.Stage(ctx, 1)
	require.NoError(t, err)
	values := make([]string, 1000)
	for i := range values {
		values[i] = fmt.Sprintf("v-%d", i)
	}
	require.ErrorIs(t, st.AddBatch(ctx, slices.Values(values)), memory.ErrFilterFull)
}

func TestStorage_Stage_CommitCanceledContext(t *testing.T) {
	t.Parallel()
	storage := memory.New(memory.WithCapacity(64))
	ctx := t.Context()

	require.NoError(t, storage.Add(ctx, "old"))
	st, err := storage.Stage(ctx, 1)
	require.NoError(t, err)

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, st.Commit(canceled), context.Canceled)

	exists, err := storage.MightExist(ctx, "old")
	require.NoError(t, err)
	require.True(t, exists, "a canceled Commit must not swap")
}

// absentProbe returns a value the storage currently reports as absent.
func absentProbe(t *testing.T, s *memory.Storage) string {
	t.Helper()
	for i := range 1000 {
		v := fmt.Sprintf("probe-%d", i)
		ok, err := s.MightExist(t.Context(), v)
		require.NoError(t, err)
		if !ok {
			return v
		}
	}
	t.Fatal("no absent probe found")
	return ""
}
