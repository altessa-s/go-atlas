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

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/internal/facade"
)

var errClose = errors.New("close failed")

// memStore is a set-backed [facade.Store]. AddBatch fails with errWrite after
// failAfter values when failAfter > 0.
type memStore struct {
	mu        sync.Mutex
	values    map[string]bool
	failAfter int
	closeErr  error
	closed    bool
}

func newMemStore() *memStore { return &memStore{values: map[string]bool{}} }

func (s *memStore) MightExist(_ context.Context, value string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[value], nil
}

func (s *memStore) Add(_ context.Context, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[value] = true
	return nil
}

func (s *memStore) AddBatch(_ context.Context, values iter.Seq[string]) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for v := range values {
		if s.failAfter > 0 && n == s.failAfter {
			return errWrite
		}
		s.values[v] = true
		n++
	}
	return nil
}

func (s *memStore) Close(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return s.closeErr
}

// unresolvedStaging reports an indeterminate commit and fails to abort until
// abortOK is set, leaving the filter fenced.
type unresolvedStaging struct {
	fakeStaging
	abortOK atomic.Bool
}

func (s *unresolvedStaging) Commit(context.Context) error { return probfilter.ErrCommitIndeterminate }

func (s *unresolvedStaging) Abort(context.Context) error {
	if !s.abortOK.Load() {
		return errors.New("redis down")
	}
	return nil
}

func TestBase_AddAndLookupObserved(t *testing.T) {
	t.Parallel()
	b := facade.NewBase(newMemStore())
	o := &recordingObserver{}
	b.SetObserver(o)

	require.NoError(t, b.Add(t.Context(), "a"))
	require.NoError(t, b.AddBatch(t.Context(), slices.Values([]string{"b", "c"})))
	found, err := b.MightExist(t.Context(), "c")
	require.NoError(t, err)
	require.True(t, found)

	require.Equal(t, 3, o.adds)
	require.Equal(t, []bool{true}, o.lookups)
}

func TestBase_FailedAddBatchReportsNothing(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	store.failAfter = 1
	b := facade.NewBase(store)
	o := &recordingObserver{}
	b.SetObserver(o)

	require.ErrorIs(t, b.AddBatch(t.Context(), slices.Values([]string{"a", "b", "c"})), errWrite)
	require.Zero(t, o.adds)
}

// TestBase_LookupFailsWhileFenced leaves the filter fenced after an
// unresolved commit: lookups and writes fail with ErrCommitIndeterminate until
// discarding the staging filter succeeds.
func TestBase_LookupFailsWhileFenced(t *testing.T) {
	t.Parallel()
	b := facade.NewBase(newMemStore())
	st := &unresolvedStaging{}
	begin := func(context.Context) (facade.StageFunc, func(context.Context) error, error) {
		return func(context.Context, int64) (facade.Staging, error) { return st, nil }, nil, nil
	}

	committed := false
	err := b.Rebuild(t.Context(), &hookLoader{values: []string{"a"}, count: 1, hookAt: -1}, begin, func() { committed = true })
	require.ErrorIs(t, err, probfilter.ErrCommitIndeterminate)
	require.False(t, committed, "a failed rebuild is not recorded")

	_, err = b.MightExist(t.Context(), "a")
	require.ErrorIs(t, err, probfilter.ErrCommitIndeterminate)
	require.ErrorIs(t, b.Add(t.Context(), "x"), probfilter.ErrCommitIndeterminate)

	st.abortOK.Store(true)
	found, err := b.MightExist(t.Context(), "x")
	require.NoError(t, err)
	require.False(t, found, "a fenced add never reached the store")
}

func TestBase_RebuildRecordsCommit(t *testing.T) {
	t.Parallel()
	b := facade.NewBase(newMemStore())
	o := &recordingObserver{}
	b.SetObserver(o)
	st := &fakeStaging{}
	begin := func(context.Context) (facade.StageFunc, func(context.Context) error, error) {
		return facade.StageWith(func(context.Context, int64) (*fakeStaging, error) { return st, nil }), nil, nil
	}

	committed := false
	require.NoError(t, b.Rebuild(t.Context(), &hookLoader{values: []string{"a"}, count: 1, hookAt: -1}, begin,
		func() { committed = true }))
	require.True(t, committed)
	require.True(t, st.committed)
	require.Equal(t, []error{nil}, o.rebuilds)
}

func TestBase_CloseJoinsErrors(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	store.closeErr = errClose
	b := facade.NewBase(store)

	require.ErrorIs(t, b.Close(t.Context()), errClose)
	require.True(t, store.closed)
	<-b.Done()
	err := b.Rebuild(t.Context(), &hookLoader{hookAt: -1}, nil, func() {})
	require.ErrorIs(t, err, probfilter.ErrFilterClosed)
}
