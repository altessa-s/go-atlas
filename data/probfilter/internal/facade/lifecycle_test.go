// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package facade_test

import (
	"context"
	"errors"
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

// commitSpy records whether Commit ran and whether a delete was in flight.
type commitSpy struct {
	fakeStaging
	inFlight      *atomic.Bool
	sawInFlight   atomic.Bool
	commitStarted chan struct{}
}

func (s *commitSpy) Commit(ctx context.Context) error {
	if s.inFlight.Load() {
		s.sawInFlight.Store(true)
	}
	close(s.commitStarted)
	return s.fakeStaging.Commit(ctx)
}

func TestCoordinator_DeleteDoesNotStraddleCommit(t *testing.T) {
	t.Parallel()
	var c facade.Coordinator
	var inFlight atomic.Bool
	spy := &commitSpy{inFlight: &inFlight, commitStarted: make(chan struct{})}

	deleteEntered := make(chan struct{})
	releaseDelete := make(chan struct{})
	deleteDone := make(chan struct{})

	// The loader starts a delete that blocks inside its storage call, then
	// finishes, so the rebuild reaches its commit while the delete is in flight.
	loader := &hookLoader{values: []string{"a"}, count: 1, hookAt: 0, hook: func() {
		go func() {
			defer close(deleteDone)
			_, err := c.Delete(t.Context(), func() (bool, error) {
				inFlight.Store(true)
				close(deleteEntered)
				<-releaseDelete
				inFlight.Store(false)
				return true, nil
			})
			require.NoError(t, err)
		}()
		<-deleteEntered
	}}

	rebuildDone := make(chan error, 1)
	go func() {
		rebuildDone <- c.Rebuild(t.Context(), loader, func(context.Context, int64) (facade.Staging, error) { return spy, nil })
	}()

	select {
	case <-spy.commitStarted:
		t.Fatal("commit ran while a delete was in flight")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseDelete)
	<-deleteDone
	require.NoError(t, <-rebuildDone)
	require.False(t, spy.sawInFlight.Load())
	require.True(t, spy.committed)
}

func TestCoordinator_CloseRefusesLaterRebuilds(t *testing.T) {
	t.Parallel()
	var c facade.Coordinator
	c.Close(t.Context())
	c.Close(t.Context()) // idempotent

	called := false
	err := c.Rebuild(t.Context(), &hookLoader{values: []string{"a"}, count: 1, hookAt: -1},
		func(context.Context, int64) (facade.Staging, error) {
			called = true
			return &fakeStaging{}, nil
		})
	require.ErrorIs(t, err, probfilter.ErrFilterClosed)
	require.False(t, called)
}

func TestCoordinator_CloseInterruptsRunningRebuild(t *testing.T) {
	t.Parallel()
	var c facade.Coordinator
	st := &fakeStaging{}
	started := make(chan struct{})
	var loaderExited atomic.Bool

	// A loader that stops silently once its context is canceled.
	loader := probfilter.DataLoaderFunc(func(ctx context.Context) iter.Seq2[string, error] {
		return func(yield func(string, error) bool) {
			close(started)
			<-ctx.Done()
			loaderExited.Store(true)
		}
	})

	rebuildDone := make(chan error, 1)
	go func() { rebuildDone <- c.Rebuild(t.Context(), loader, stageWith(st)) }()
	<-started

	c.Close(t.Context())
	require.True(t, loaderExited.Load(), "Close must wait for the running rebuild")
	require.ErrorIs(t, <-rebuildDone, probfilter.ErrFilterClosed)
	require.False(t, st.committed, "no commit after Close")
}

func TestCoordinator_CloseWaitsForQueuedRebuild(t *testing.T) {
	t.Parallel()
	var c facade.Coordinator
	release := make(chan struct{})
	started := make(chan struct{})

	blocking := probfilter.DataLoaderFunc(func(ctx context.Context) iter.Seq2[string, error] {
		return func(yield func(string, error) bool) {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
			}
			yield("a", nil)
		}
	})

	first := make(chan error, 1)
	go func() { first <- c.Rebuild(t.Context(), blocking, stageWith(&fakeStaging{})) }()
	<-started

	queuedStage := &fakeStaging{}
	queued := make(chan error, 1)
	go func() {
		queued <- c.Rebuild(t.Context(), &hookLoader{values: []string{"b"}, count: 1, hookAt: -1}, stageWith(queuedStage))
	}()

	c.Close(t.Context())
	close(release)
	require.ErrorIs(t, <-first, probfilter.ErrFilterClosed)
	require.ErrorIs(t, <-queued, probfilter.ErrFilterClosed)
	require.False(t, queuedStage.committed)
}

// indeterminateStaging reports an unknown commit outcome; Abort fails while
// abortErr is set.
type indeterminateStaging struct {
	fakeStaging
	abortErr atomic.Pointer[error]
	aborts   atomic.Int32
}

func (s *indeterminateStaging) Commit(context.Context) error {
	return probfilter.ErrCommitIndeterminate
}

func (s *indeterminateStaging) Abort(context.Context) error {
	s.aborts.Add(1)
	if p := s.abortErr.Load(); p != nil {
		return *p
	}
	return nil
}

func TestCoordinator_IndeterminateCommit_AbortSucceeds(t *testing.T) {
	t.Parallel()
	var c facade.Coordinator
	st := &indeterminateStaging{}

	err := c.Rebuild(t.Context(), &hookLoader{values: []string{"a"}, count: 1, hookAt: -1},
		func(context.Context, int64) (facade.Staging, error) { return st, nil })
	require.ErrorIs(t, err, probfilter.ErrCommitIndeterminate)
	require.Equal(t, int32(1), st.aborts.Load(), "the replacement is discarded so a delayed promotion is harmless")
	require.NoError(t, c.Fence(t.Context()), "a discarded replacement does not fence the filter")
	require.NoError(t, c.Add(t.Context(), "x", func() error { return nil }))
}

func TestCoordinator_IndeterminateCommit_FencesUntilDiscarded(t *testing.T) {
	t.Parallel()
	var c facade.Coordinator
	st := &indeterminateStaging{}
	errDown := errors.New("redis down")
	st.abortErr.Store(&errDown)

	err := c.Rebuild(t.Context(), &hookLoader{values: []string{"a"}, count: 1, hookAt: -1},
		func(context.Context, int64) (facade.Staging, error) { return st, nil })
	require.ErrorIs(t, err, probfilter.ErrCommitIndeterminate)

	// Fenced: writes, deletes, lookups (via Fence) and rebuilds are refused,
	// and the live write never runs.
	wrote := false
	require.ErrorIs(t, c.Add(t.Context(), "x", func() error { wrote = true; return nil }), probfilter.ErrCommitIndeterminate)
	require.ErrorIs(t, c.AddBatch(t.Context(), slices.Values([]string{"y"}), func(iter.Seq[string]) error { wrote = true; return nil }),
		probfilter.ErrCommitIndeterminate)
	_, err = c.Delete(t.Context(), func() (bool, error) { wrote = true; return true, nil })
	require.ErrorIs(t, err, probfilter.ErrCommitIndeterminate)
	require.ErrorIs(t, c.Fence(t.Context()), probfilter.ErrCommitIndeterminate)
	require.False(t, wrote)
	require.ErrorIs(t, c.Close(t.Context()), probfilter.ErrCommitIndeterminate,
		"Close must not promise that no promotion can happen")

	// Once discarding succeeds the fence lifts.
	st.abortErr.Store(nil)
	require.NoError(t, c.Fence(t.Context()))
	require.NoError(t, c.Add(t.Context(), "x", func() error { wrote = true; return nil }))
	require.True(t, wrote)
}

// blockingIndeterminateStaging blocks in Commit until released, then reports
// an unknown outcome; its Abort always fails.
type blockingIndeterminateStaging struct {
	fakeStaging
	commitEntered chan struct{}
	release       chan struct{}
}

func (s *blockingIndeterminateStaging) Commit(context.Context) error {
	close(s.commitEntered)
	<-s.release
	return probfilter.ErrCommitIndeterminate
}

func (s *blockingIndeterminateStaging) Abort(context.Context) error {
	return errors.New("redis down")
}

// TestCoordinator_WriterQueuedBehindCommitSeesFence queues a writer while the
// commit holds the exclusive gate; the commit then turns out unresolved. The
// writer must observe the fence and never run its live write.
func TestCoordinator_WriterQueuedBehindCommitSeesFence(t *testing.T) {
	t.Parallel()
	var c facade.Coordinator
	st := &blockingIndeterminateStaging{commitEntered: make(chan struct{}), release: make(chan struct{})}

	rebuilt := make(chan error, 1)
	go func() {
		rebuilt <- c.Rebuild(t.Context(), &hookLoader{values: []string{"a"}, count: 1, hookAt: -1},
			func(context.Context, int64) (facade.Staging, error) { return st, nil })
	}()
	<-st.commitEntered

	var wrote atomic.Bool
	added := make(chan error, 1)
	go func() { added <- c.Add(t.Context(), "x", func() error { wrote.Store(true); return nil }) }()
	deleted := make(chan error, 1)
	go func() {
		_, err := c.Delete(t.Context(), func() (bool, error) { wrote.Store(true); return true, nil })
		deleted <- err
	}()

	time.Sleep(20 * time.Millisecond) // let both writers queue on the gate
	close(st.release)

	require.ErrorIs(t, <-rebuilt, probfilter.ErrCommitIndeterminate)
	require.ErrorIs(t, <-added, probfilter.ErrCommitIndeterminate)
	require.ErrorIs(t, <-deleted, probfilter.ErrCommitIndeterminate)
	require.False(t, wrote.Load(), "a fenced filter must not be written")
}

func TestCoordinator_DoneClosedByClose(t *testing.T) {
	t.Parallel()
	var c facade.Coordinator
	done := c.Done()
	select {
	case <-done:
		t.Fatal("Done closed before Close")
	default:
	}

	var wg sync.WaitGroup
	for range 4 { // concurrent Close calls must not double-close
		wg.Go(func() { require.NoError(t, c.Close(t.Context())) })
	}
	wg.Wait()
	<-done
	<-c.Done()
}
