// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bloom_test

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"
)

var errLoad = errors.New("load failed")

// hookLoader streams values and calls hook after yielding the value at
// index hookAt, so a test can act while the rebuild is mid-stream. When
// failAfter > 0 the stream fails after that many values.
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

func (l *hookLoader) Count(_ context.Context) (int64, error) { return l.count, nil }

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

func TestFilter_Rebuild_LookupsDuringRebuildSeeOldContents(t *testing.T) {
	t.Parallel()

	for _, count := range []int64{-1, 50} {
		t.Run(fmt.Sprintf("count=%d", count), func(t *testing.T) {
			t.Parallel()
			f := newTestFilter()
			old := keys("old", 50)
			require.NoError(t, f.Rebuild(t.Context(), &hookLoader{values: old, count: int64(len(old)), hookAt: -1}))

			fresh := keys("new", 50)
			var missing []string
			loader := &hookLoader{values: fresh, count: count, hookAt: 10, hook: func() {
				for _, v := range old {
					if ok, _ := f.MightExist(t.Context(), v); !ok {
						missing = append(missing, v)
					}
				}
			}}
			require.NoError(t, f.Rebuild(t.Context(), loader))
			require.Empty(t, missing, "old keys must stay visible while the rebuild runs")
			requireAll(t, f, fresh)
		})
	}
}

func TestFilter_Rebuild_FailureKeepsOldContents(t *testing.T) {
	t.Parallel()

	for _, count := range []int64{-1, 50} {
		t.Run(fmt.Sprintf("count=%d", count), func(t *testing.T) {
			t.Parallel()
			f := newTestFilter()
			old := keys("old", 50)
			require.NoError(t, f.Rebuild(t.Context(), &hookLoader{values: old, count: int64(len(old)), hookAt: -1}))
			last := f.LastRebuild()

			err := f.Rebuild(t.Context(), &hookLoader{values: keys("new", 50), count: count, hookAt: -1, failAfter: 20})
			require.ErrorIs(t, err, errLoad)
			requireAll(t, f, old)
			require.Equal(t, last, f.LastRebuild(), "a failed rebuild must not move LastRebuild")
		})
	}
}

func TestFilter_Rebuild_ConcurrentAddsSurvive(t *testing.T) {
	t.Parallel()

	for _, count := range []int64{-1, 200} {
		t.Run(fmt.Sprintf("count=%d", count), func(t *testing.T) {
			t.Parallel()
			f := newTestFilter()
			added := keys("added", 100)

			// Adds are issued from goroutines while the loader is mid-stream and
			// complete before the stream continues, so they are guaranteed to
			// land inside the rebuild window.
			loader := &hookLoader{values: keys("src", 200), count: count, hookAt: 150, hook: func() {
				var wg sync.WaitGroup
				for _, v := range added {
					wg.Go(func() { require.NoError(t, f.Add(t.Context(), v)) })
				}
				wg.Wait()
			}}
			require.NoError(t, f.Rebuild(t.Context(), loader))
			requireAll(t, f, added)
			requireAll(t, f, loader.values)
		})
	}
}

func TestFilter_Rebuild_Closed(t *testing.T) {
	t.Parallel()
	f := newTestFilter()
	require.NoError(t, f.Close(t.Context()))
	err := f.Rebuild(t.Context(), &hookLoader{values: []string{"a"}, count: 1, hookAt: -1})
	require.ErrorIs(t, err, probfilter.ErrFilterClosed)
	require.True(t, f.LastRebuild().IsZero())
}

func TestFilter_Rebuild_CanceledLeavesContents(t *testing.T) {
	t.Parallel()
	f := newTestFilter()
	old := keys("old", 20)
	require.NoError(t, f.Rebuild(t.Context(), &hookLoader{values: old, count: 20, hookAt: -1}))
	last := f.LastRebuild()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// The loader ignores cancellation and finishes normally; the rebuild must
	// still refuse to commit once ctx is canceled.
	err := f.Rebuild(ctx, &hookLoader{values: keys("new", 20), count: 20, hookAt: 5, hook: cancel})
	require.ErrorIs(t, err, context.Canceled)
	requireAll(t, f, old)
	require.Equal(t, last, f.LastRebuild())
}
