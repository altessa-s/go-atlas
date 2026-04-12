// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cuckoo_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
)

func TestNew_WithLogger(t *testing.T) {
	storage := memory.New(memory.WithCapacity(100))
	f := cuckoo.New(storage, cuckoo.WithLogger(slog.Default()))
	require.NotNil(t, f, "New(WithLogger) returned nil")
}

func TestFilter_Stats_Complete(t *testing.T) {
	storage := memory.New(memory.WithCapacity(100))
	f := cuckoo.New(storage)
	ctx := t.Context()

	// Add some items
	_ = f.Add(ctx, "a")
	_ = f.Add(ctx, "b")

	stats, err := f.Stats(ctx)
	require.NoError(t, err)
	require.NotNil(t, stats, "Stats() returned nil")
}

func TestFilter_DeleteAndRecheck(t *testing.T) {
	storage := memory.New(memory.WithCapacity(100))
	f := cuckoo.New(storage)
	ctx := t.Context()

	_ = f.Add(ctx, "item1")

	ok, err := f.Delete(ctx, "item1")
	require.NoError(t, err)
	require.True(t, ok, "Delete() should return true for existing item")

	// After delete, MightExist should return false
	exists, err := f.MightExist(ctx, "item1")
	require.NoError(t, err)
	require.False(t, exists, "MightExist() should return false after Delete")
}

func TestFilter_AddBatch_Multiple(t *testing.T) {
	storage := memory.New(memory.WithCapacity(1000))
	f := cuckoo.New(storage)
	ctx := t.Context()

	items := func(yield func(string) bool) {
		for i := range 100 {
			if !yield("item-" + string(rune('a'+i%26))) {
				return
			}
		}
	}

	require.NoError(t, f.AddBatch(ctx, items))
}
