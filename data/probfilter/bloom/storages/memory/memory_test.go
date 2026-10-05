// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	memory "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/memory"
)

func TestStorage_AddAndMightExist(t *testing.T) {
	storage := memory.New(memory.WithExpectedItems(1000))
	ctx := t.Context()

	err := storage.Add(ctx, "hello")
	require.NoError(t, err)

	exists, err := storage.MightExist(ctx, "hello")
	require.NoError(t, err)
	require.True(t, exists, "Expected 'hello' to exist in filter")
}

func TestStorage_MightExist_NotAdded(t *testing.T) {
	storage := memory.New(memory.WithExpectedItems(1000))
	ctx := t.Context()

	exists, err := storage.MightExist(ctx, "notadded")
	require.NoError(t, err)
	require.False(t, exists, "Expected 'notadded' to not exist in filter")
}

func TestStorage_AddBatch(t *testing.T) {
	storage := memory.New(memory.WithExpectedItems(1000))
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

func TestStorage_Stage_CommitReplacesContents(t *testing.T) {
	t.Parallel()
	storage := memory.New(memory.WithExpectedItems(1000))
	ctx := t.Context()

	require.NoError(t, storage.Add(ctx, "old"))

	st, err := storage.Stage(ctx, 100)
	require.NoError(t, err)
	require.NoError(t, st.AddBatch(ctx, slices.Values([]string{"new"})))

	// The live filter is untouched until Commit.
	exists, err := storage.MightExist(ctx, "old")
	require.NoError(t, err)
	require.True(t, exists, "live contents must stay visible while staging")
	exists, err = storage.MightExist(ctx, "new")
	require.NoError(t, err)
	require.False(t, exists, "staged contents must not be visible before Commit")

	require.NoError(t, st.Commit(ctx))

	exists, err = storage.MightExist(ctx, "new")
	require.NoError(t, err)
	require.True(t, exists)
	exists, err = storage.MightExist(ctx, "old")
	require.NoError(t, err)
	require.False(t, exists, "Commit must replace the live contents")

	stats, err := storage.Stats(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.ItemCount)
}

func TestStorage_Stage_AbortKeepsContents(t *testing.T) {
	t.Parallel()
	storage := memory.New(memory.WithExpectedItems(1000))
	ctx := t.Context()

	require.NoError(t, storage.Add(ctx, "old"))

	st, err := storage.Stage(ctx, 0)
	require.NoError(t, err)
	require.NoError(t, st.AddBatch(ctx, slices.Values([]string{"new"})))
	require.NoError(t, st.Abort(ctx))

	exists, err := storage.MightExist(ctx, "old")
	require.NoError(t, err)
	require.True(t, exists)
	exists, err = storage.MightExist(ctx, "new")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestStorage_Stage_CommitCanceledContext(t *testing.T) {
	t.Parallel()
	storage := memory.New(memory.WithExpectedItems(1000))
	ctx := t.Context()

	require.NoError(t, storage.Add(ctx, "old"))
	st, err := storage.Stage(ctx, 10)
	require.NoError(t, err)

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, st.Commit(canceled), context.Canceled)

	exists, err := storage.MightExist(ctx, "old")
	require.NoError(t, err)
	require.True(t, exists, "a canceled Commit must not swap")
}

func TestStorage_Stats(t *testing.T) {
	storage := memory.New(memory.WithExpectedItems(1000))
	ctx := t.Context()

	for i := range 10 {
		err := storage.Add(ctx, string(rune('a'+i)))
		require.NoError(t, err)
	}

	stats, err := storage.Stats(ctx)
	require.NoError(t, err)

	require.True(t, stats.Capacity > 0, "Expected capacity > 0, got %d", stats.Capacity)
	require.True(t, stats.ItemCount > 0, "Expected itemCount > 0, got %d", stats.ItemCount)

	require.Equal(t, "memory", stats.StorageType)
}

func TestStorage_LastRebuild(t *testing.T) {
	storage := memory.New(memory.WithExpectedItems(1000))

	initialTime := storage.LastRebuild()
	require.True(t, initialTime.IsZero(), "Expected initial LastRebuild to be zero, got %v", initialTime)

	now := time.Now()
	storage.SetLastRebuild(now)

	lastRebuild := storage.LastRebuild()
	require.True(t, lastRebuild.Equal(now), "Expected LastRebuild to be %v, got %v", now, lastRebuild)
}

func TestStorage_Close(t *testing.T) {
	storage := memory.New(memory.WithExpectedItems(1000))
	ctx := t.Context()

	err := storage.Close(ctx)
	require.NoError(t, err)
}
