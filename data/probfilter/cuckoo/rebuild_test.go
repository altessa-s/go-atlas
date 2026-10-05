// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cuckoo_test

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
)

var errLoad = errors.New("load failed")

// hookLoader streams values and runs hook after yielding the value at hookAt;
// failAfter > 0 makes the stream fail at that index.
type hookLoader struct {
	values    []string
	count     int64
	hookAt    int
	hook      func()
	failAfter int
}

func (l *hookLoader) StreamValues(_ context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for i, v := range l.values {
			if l.failAfter > 0 && i == l.failAfter {
				yield("", errLoad)
				return
			}
			if !yield(v, nil) {
				return
			}
			if i == l.hookAt && l.hook != nil {
				l.hook()
			}
		}
	}
}

func (l *hookLoader) Count(context.Context) (int64, error) { return l.count, nil }

func keys(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s-%d", prefix, i)
	}
	return out
}

func requireAll(t *testing.T, f probfilter.Filter, values []string) {
	t.Helper()
	for _, v := range values {
		ok, err := f.MightExist(t.Context(), v)
		require.NoError(t, err)
		require.True(t, ok, "MightExist(%q) = false", v)
	}
}

func TestFilter_ImplementsRebuildable(t *testing.T) {
	t.Parallel()
	var f probfilter.Filter = cuckoo.New(memory.New())
	_, ok := f.(probfilter.RebuildableFilter)
	require.True(t, ok)
}

func TestFilter_Rebuild_ReplacesContents(t *testing.T) {
	t.Parallel()

	for _, count := range []int64{-1, 300} {
		t.Run(fmt.Sprintf("count=%d", count), func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			f := cuckoo.New(memory.New(memory.WithCapacity(64)))
			require.True(t, f.LastRebuild().IsZero())

			stale := keys("stale", 50)
			require.NoError(t, f.AddBatch(ctx, slices.Values(stale)))
			values := keys("v", 300) // more than the configured capacity
			before := time.Now()
			require.NoError(t, f.Rebuild(ctx, &hookLoader{values: values, count: count, hookAt: -1}))

			requireAll(t, f, values)
			// Values missing from the source are dropped; a few may still
			// match as false positives (8-bit fingerprints).
			stillPresent := 0
			for _, v := range stale {
				ok, err := f.MightExist(ctx, v)
				require.NoError(t, err)
				if ok {
					stillPresent++
				}
			}
			require.Less(t, stillPresent, 10, "values missing from the source are dropped")
			require.False(t, f.LastRebuild().Before(before))

			stats, err := f.Stats(ctx)
			require.NoError(t, err)
			require.Equal(t, f.LastRebuild(), stats.LastRebuild)
		})
	}
}

func TestFilter_Rebuild_FailureKeepsContents(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	f := cuckoo.New(memory.New(memory.WithCapacity(1000)))
	old := keys("old", 50)
	require.NoError(t, f.Rebuild(ctx, &hookLoader{values: old, count: 50, hookAt: -1}))
	last := f.LastRebuild()

	err := f.Rebuild(ctx, &hookLoader{values: keys("new", 50), count: 50, hookAt: -1, failAfter: 10})
	require.ErrorIs(t, err, errLoad)
	requireAll(t, f, old)
	require.Equal(t, last, f.LastRebuild())
}

func TestFilter_Rebuild_LookupsAndAddsDuringRebuild(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	f := cuckoo.New(memory.New(memory.WithCapacity(1000)))
	old := keys("old", 50)
	require.NoError(t, f.Rebuild(ctx, &hookLoader{values: old, count: 50, hookAt: -1}))

	added := keys("added", 50)
	var missing []string
	loader := &hookLoader{values: keys("src", 100), count: -1, hookAt: 50, hook: func() {
		for _, v := range old {
			if ok, _ := f.MightExist(ctx, v); !ok {
				missing = append(missing, v)
			}
		}
		var wg sync.WaitGroup
		for _, v := range added {
			wg.Go(func() { require.NoError(t, f.Add(ctx, v)) })
		}
		wg.Go(func() { require.NoError(t, f.AddBatch(ctx, slices.Values([]string{"b1", "b2"}))) })
		wg.Wait()
	}}

	require.NoError(t, f.Rebuild(ctx, loader))
	require.Empty(t, missing, "old contents stay visible during the rebuild")
	requireAll(t, f, added)
	requireAll(t, f, []string{"b1", "b2"})
	requireAll(t, f, loader.values)
}

func TestFilter_Rebuild_DeleteDuringRebuildNotReplayed(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	f := cuckoo.New(memory.New(memory.WithCapacity(1000)))
	require.NoError(t, f.Add(ctx, "gone"))

	// The source still lists "gone" when the loader runs; the delete issued
	// during the rebuild hits only the live filter, so "gone" reappears as a
	// possible member (a false positive, never a false negative).
	loader := &hookLoader{values: []string{"gone", "kept"}, count: 2, hookAt: 1, hook: func() {
		deleted, err := f.Delete(ctx, "gone")
		require.NoError(t, err)
		require.True(t, deleted)
		ok, err := f.MightExist(ctx, "gone")
		require.NoError(t, err)
		require.False(t, ok, "the delete applies to the live filter immediately")
	}}

	require.NoError(t, f.Rebuild(ctx, loader))
	requireAll(t, f, []string{"gone", "kept"})
}

func TestFilter_Rebuild_Closed(t *testing.T) {
	t.Parallel()
	f := cuckoo.New(memory.New())
	require.NoError(t, f.Close(t.Context()))
	err := f.Rebuild(t.Context(), &hookLoader{values: []string{"a"}, count: 1, hookAt: -1})
	require.ErrorIs(t, err, probfilter.ErrFilterClosed)
	require.True(t, f.LastRebuild().IsZero())
}
