// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package facade_test

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/internal/facade"
)

var (
	errLoad   = errors.New("load failed")
	errStage  = errors.New("stage failed")
	errCommit = errors.New("commit failed")
	errWrite  = errors.New("write failed")
)

// fakeStaging records what a rebuild put into the replacement filter.
type fakeStaging struct {
	mu        sync.Mutex
	values    []string
	addErr    error
	commitErr error
	committed bool
	aborted   bool
	expected  int64
}

func (s *fakeStaging) AddBatch(_ context.Context, values iter.Seq[string]) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for v := range values {
		if s.addErr != nil {
			return s.addErr
		}
		s.values = append(s.values, v)
	}
	return nil
}

func (s *fakeStaging) Commit(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.commitErr != nil {
		return s.commitErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.committed = true
	return nil
}

func (s *fakeStaging) Abort(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aborted = true
	return nil
}

func (s *fakeStaging) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.values)
}

func stageWith(st *fakeStaging) facade.StageFunc {
	return func(_ context.Context, expected int64) (facade.Staging, error) {
		st.expected = expected
		return st, nil
	}
}

// hookLoader yields values and runs hook after the value at hookAt. It stops
// silently (no error) when ctx is canceled, as DataLoader permits; failAt > 0
// yields errLoad at that index.
type hookLoader struct {
	values []string
	count  int64
	hookAt int
	hook   func()
	failAt int
}

func (l *hookLoader) StreamValues(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for i, v := range l.values {
			if ctx.Err() != nil {
				return
			}
			if l.failAt > 0 && i == l.failAt {
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

func TestCoordinator_Rebuild_Success(t *testing.T) {
	t.Parallel()

	for _, count := range []int64{-1, 3} {
		t.Run(fmt.Sprintf("count=%d", count), func(t *testing.T) {
			t.Parallel()
			var c facade.Coordinator
			st := &fakeStaging{}
			loader := &hookLoader{values: []string{"a", "b", "c"}, count: count, hookAt: -1}

			require.NoError(t, c.Rebuild(t.Context(), loader, stageWith(st)))
			require.True(t, st.committed)
			require.False(t, st.aborted)
			require.Equal(t, []string{"a", "b", "c"}, st.snapshot())
			require.Equal(t, int64(3), st.expected, "the replacement is sized for the loaded values")
		})
	}
}

func TestCoordinator_Rebuild_ReplaysAddsMadeDuringRebuild(t *testing.T) {
	t.Parallel()

	for _, count := range []int64{-1, 100} {
		t.Run(fmt.Sprintf("count=%d", count), func(t *testing.T) {
			t.Parallel()
			var c facade.Coordinator
			st := &fakeStaging{}
			var live sync.Map
			added := keys("added", 50)

			loader := &hookLoader{values: keys("src", 100), count: count, hookAt: 40, hook: func() {
				var wg sync.WaitGroup
				for _, v := range added {
					wg.Go(func() {
						require.NoError(t, c.Add(t.Context(), v, func() error { live.Store(v, true); return nil }))
					})
				}
				wg.Go(func() {
					require.NoError(t, c.AddBatch(t.Context(), slices.Values([]string{"batch-1", "batch-2"}),
						func(values iter.Seq[string]) error {
							for v := range values {
								live.Store(v, true)
							}
							return nil
						}))
				})
				wg.Wait()
			}}

			require.NoError(t, c.Rebuild(t.Context(), loader, stageWith(st)))
			got := st.snapshot()
			for _, v := range append(slices.Clone(added), "batch-1", "batch-2") {
				require.Contains(t, got, v)
				_, ok := live.Load(v)
				require.True(t, ok, "the live write must still happen")
			}
		})
	}
}

func TestCoordinator_Add_NotJournaledOutsideRebuild(t *testing.T) {
	t.Parallel()
	var c facade.Coordinator
	require.NoError(t, c.Add(t.Context(), "before", func() error { return nil }))

	st := &fakeStaging{}
	require.NoError(t, c.Rebuild(t.Context(), &hookLoader{values: []string{"src"}, count: 1, hookAt: -1}, stageWith(st)))
	require.Equal(t, []string{"src"}, st.snapshot())

	// Journaling stops with the rebuild.
	require.NoError(t, c.Add(t.Context(), "after", func() error { return nil }))
	st2 := &fakeStaging{}
	require.NoError(t, c.Rebuild(t.Context(), &hookLoader{values: []string{"src"}, count: 1, hookAt: -1}, stageWith(st2)))
	require.Equal(t, []string{"src"}, st2.snapshot())
}

// TestCoordinator_JournalOrderMatchesWriteOrder checks that, during a rebuild,
// the journal records adds in the exact order the live filter executed them.
func TestCoordinator_JournalOrderMatchesWriteOrder(t *testing.T) {
	t.Parallel()
	var c facade.Coordinator
	st := &fakeStaging{}

	var liveMu sync.Mutex
	var liveOrder []string
	write := func(v string) func() error {
		return func() error {
			// Yield inside the write to give racing writers a chance to
			// interleave between the live write and the journal append.
			time.Sleep(time.Microsecond)
			liveMu.Lock()
			liveOrder = append(liveOrder, v)
			liveMu.Unlock()
			return nil
		}
	}

	loader := &hookLoader{values: []string{"src"}, count: -1, hookAt: 0, hook: func() {
		var wg sync.WaitGroup
		for _, v := range keys("w", 200) {
			wg.Go(func() { require.NoError(t, c.Add(t.Context(), v, write(v))) })
		}
		wg.Wait()
	}}

	require.NoError(t, c.Rebuild(t.Context(), loader, stageWith(st)))
	got := st.snapshot()
	require.Equal(t, "src", got[0])
	require.Equal(t, liveOrder, got[1:], "replay order must equal live write order")
}

func TestCoordinator_FailedWritesAreStillJournaled(t *testing.T) {
	t.Parallel()
	var c facade.Coordinator
	st := &fakeStaging{}

	loader := &hookLoader{values: []string{"src"}, count: 1, hookAt: 0, hook: func() {
		require.ErrorIs(t, c.Add(t.Context(), "single", func() error { return errWrite }), errWrite)
		err := c.AddBatch(t.Context(), slices.Values([]string{"p1", "p2", "never"}), func(values iter.Seq[string]) error {
			n := 0
			for range values {
				n++
				if n == 2 {
					return errWrite // consumed p1 and p2, then failed
				}
			}
			return nil
		})
		require.ErrorIs(t, err, errWrite)
	}}

	require.NoError(t, c.Rebuild(t.Context(), loader, stageWith(st)))
	require.Equal(t, []string{"src", "single", "p1", "p2"}, st.snapshot(),
		"values a failed write may have reached must be in the replacement")
}

func TestCoordinator_Rebuild_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		count      int64
		failAt     int
		stageErr   error
		addErr     error
		commitErr  error
		wantErr    error
		wantAbort  bool
		wantStaged bool
	}{
		{name: "loader error with count", count: 5, failAt: 2, wantErr: errLoad, wantAbort: true, wantStaged: true},
		{name: "loader error without count", count: -1, failAt: 2, wantErr: errLoad},
		{name: "stage error", count: 5, stageErr: errStage, wantErr: errStage},
		{name: "add error", count: 5, addErr: errWrite, wantErr: errWrite, wantAbort: true, wantStaged: true},
		{name: "commit error", count: 5, commitErr: errCommit, wantErr: errCommit, wantAbort: true, wantStaged: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var c facade.Coordinator
			st := &fakeStaging{addErr: tc.addErr, commitErr: tc.commitErr}
			staged := false
			stage := func(context.Context, int64) (facade.Staging, error) {
				if tc.stageErr != nil {
					return nil, tc.stageErr
				}
				staged = true
				return st, nil
			}

			loader := &hookLoader{values: keys("v", 5), count: tc.count, hookAt: -1, failAt: tc.failAt}
			err := c.Rebuild(t.Context(), loader, stage)
			require.ErrorIs(t, err, tc.wantErr)
			require.False(t, st.committed)
			require.Equal(t, tc.wantAbort, st.aborted)
			require.Equal(t, tc.wantStaged, staged)

			// The coordinator is reusable after a failure and no longer journals.
			require.NoError(t, c.Add(t.Context(), "x", func() error { return nil }))
			st2 := &fakeStaging{}
			require.NoError(t, c.Rebuild(t.Context(), &hookLoader{values: []string{"ok"}, count: 1, hookAt: -1}, stageWith(st2)))
			require.Equal(t, []string{"ok"}, st2.snapshot())
		})
	}
}

func TestCoordinator_Rebuild_CanceledLoaderStopsSilently(t *testing.T) {
	t.Parallel()

	for _, count := range []int64{-1, 10} {
		t.Run(fmt.Sprintf("count=%d", count), func(t *testing.T) {
			t.Parallel()
			var c facade.Coordinator
			st := &fakeStaging{}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			// The loader cancels mid-stream and then stops without an error.
			loader := &hookLoader{values: keys("v", 10), count: count, hookAt: 3, hook: cancel}
			err := c.Rebuild(ctx, loader, stageWith(st))
			require.ErrorIs(t, err, context.Canceled)
			require.False(t, st.committed, "a partial snapshot must never commit")
			if count > 0 {
				require.True(t, st.aborted)
			}
		})
	}
}

func TestCoordinator_Rebuild_CanceledBeforeStart(t *testing.T) {
	t.Parallel()
	var c facade.Coordinator
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	called := false
	err := c.Rebuild(ctx, &hookLoader{hookAt: -1}, func(context.Context, int64) (facade.Staging, error) {
		called = true
		return &fakeStaging{}, nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, called)
}

func TestCoordinator_Rebuild_Serialized(t *testing.T) {
	t.Parallel()
	var c facade.Coordinator
	var running, maxRunning atomic.Int32

	loader := probfilter.DataLoaderFunc(func(context.Context) iter.Seq2[string, error] {
		return func(yield func(string, error) bool) {
			n := running.Add(1)
			for {
				m := maxRunning.Load()
				if n <= m || maxRunning.CompareAndSwap(m, n) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			running.Add(-1)
			yield("v", nil)
		}
	})

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			require.NoError(t, c.Rebuild(t.Context(), loader, stageWith(&fakeStaging{})))
		})
	}
	wg.Wait()
	require.Equal(t, int32(1), maxRunning.Load(), "rebuilds of one filter must not overlap")
}

// recordingObserver captures observer callbacks.
type recordingObserver struct {
	mu       sync.Mutex
	lookups  []bool
	errs     []error
	adds     int
	rebuilds []error
}

func (o *recordingObserver) ObserveLookup(found bool, err error, _ time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.lookups = append(o.lookups, found)
	o.errs = append(o.errs, err)
}

func (o *recordingObserver) ObserveAdd(n int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.adds += n
}

func (o *recordingObserver) ObserveRebuild(_ time.Duration, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.rebuilds = append(o.rebuilds, err)
}

func TestObserverSlot(t *testing.T) {
	t.Parallel()
	var slot facade.ObserverSlot
	require.Nil(t, slot.Load())

	// No observer: operations still run.
	found, err := slot.Lookup(func() (bool, error) { return true, nil })
	require.NoError(t, err)
	require.True(t, found)
	slot.Added(3)
	require.ErrorIs(t, slot.Rebuild(func() error { return errLoad }), errLoad)

	o := &recordingObserver{}
	slot.Set(o)
	require.Same(t, o, slot.Load())

	_, _ = slot.Lookup(func() (bool, error) { return true, nil })
	_, err = slot.Lookup(func() (bool, error) { return false, errLoad })
	require.ErrorIs(t, err, errLoad)
	slot.Added(2)
	require.NoError(t, slot.Rebuild(func() error { return nil }))
	require.ErrorIs(t, slot.Rebuild(func() error { return errLoad }), errLoad)

	require.Equal(t, []bool{true, false}, o.lookups)
	require.Equal(t, []error{nil, errLoad}, o.errs)
	require.Equal(t, 2, o.adds)
	require.Equal(t, []error{nil, errLoad}, o.rebuilds)

	slot.Set(nil)
	require.Nil(t, slot.Load())
	slot.Added(5)
	require.Equal(t, 2, o.adds, "a removed observer receives nothing")
}
