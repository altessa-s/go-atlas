// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cuckoo_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter/cuckoo"
	"github.com/altessa-s/go-atlas/cache/probfilter/cuckoo/storages"
	"github.com/altessa-s/go-atlas/cache/probfilter/cuckoo/storages/memory"
	"github.com/altessa-s/go-atlas/cache/probfilter/stats"
)

type countingObserver struct {
	mu       sync.Mutex
	lookups  int
	added    int
	rebuilds int
}

func (o *countingObserver) ObserveLookup(bool, error, time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.lookups++
}

func (o *countingObserver) ObserveAdd(n int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.added += n
}

func (o *countingObserver) ObserveRebuild(time.Duration, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.rebuilds++
}

func TestFilter_ObserverAndDone(t *testing.T) {
	t.Parallel()
	f := cuckoo.New(memory.New())
	obs := &countingObserver{}
	f.SetObserver(obs)

	require.NoError(t, f.Add(t.Context(), "a"))
	_, err := f.MightExist(t.Context(), "a")
	require.NoError(t, err)
	require.NoError(t, f.Rebuild(t.Context(), &hookLoader{values: []string{"x"}, count: 1, hookAt: -1}))

	require.Equal(t, 1, obs.lookups)
	require.Equal(t, 1, obs.added)
	require.Equal(t, 1, obs.rebuilds)

	select {
	case <-f.Done():
		require.Fail(t, "Done must stay open until Close")
	default:
	}
	require.NoError(t, f.Close(t.Context()))
	select {
	case <-f.Done():
	case <-time.After(5 * time.Second):
		require.Fail(t, "Done must close after Close")
	}
}

func TestFilter_RebuildCommittedWithoutReporter(t *testing.T) {
	t.Parallel()
	f := cuckoo.New(memory.New())
	committed, err := f.RebuildCommitted(t.Context())
	require.NoError(t, err)
	require.False(t, committed, "no rebuild has run yet")

	require.NoError(t, f.Rebuild(t.Context(), &hookLoader{values: []string{"x"}, count: 1, hookAt: -1}))
	committed, err = f.RebuildCommitted(t.Context())
	require.NoError(t, err)
	require.True(t, committed)
	require.False(t, f.LastRebuild().IsZero())

	st, err := f.Stats(t.Context())
	require.NoError(t, err)
	require.Equal(t, f.LastRebuild(), st.LastRebuild)
}

// exclusiveStorage adds a rebuild lease and a commit reporter to the memory
// storage, as a shared backend would.
type exclusiveStorage struct {
	storages.Storage
	beginErr  error
	released  atomic.Int32
	committed bool
}

type lease struct {
	s *exclusiveStorage
}

func (l lease) Stage(ctx context.Context, expected int64) (storages.Staging, error) {
	return l.s.Storage.Stage(ctx, expected)
}

func (l lease) Release(context.Context) error {
	l.s.released.Add(1)
	return nil
}

func (s *exclusiveStorage) BeginRebuild(context.Context) (storages.RebuildLease, error) {
	if s.beginErr != nil {
		return nil, s.beginErr
	}
	return lease{s: s}, nil
}

func (s *exclusiveStorage) RebuildCommitted(context.Context) (bool, error) {
	return s.committed, nil
}

func TestFilter_RebuildUsesExclusiveLease(t *testing.T) {
	t.Parallel()
	st := &exclusiveStorage{Storage: memory.New(), committed: true}
	f := cuckoo.New(st)

	require.NoError(t, f.Rebuild(t.Context(), &hookLoader{values: []string{"leased"}, count: 1, hookAt: -1}))
	requireAll(t, f, []string{"leased"})
	require.EqualValues(t, 1, st.released.Load(), "the lease is released after the rebuild")

	committed, err := f.RebuildCommitted(t.Context())
	require.NoError(t, err)
	require.True(t, committed, "the storage's own report wins")
}

func TestFilter_RebuildFailsWhenLeaseIsHeld(t *testing.T) {
	t.Parallel()
	errBusy := errors.New("rebuild in progress elsewhere")
	st := &exclusiveStorage{Storage: memory.New(), beginErr: errBusy}
	f := cuckoo.New(st)

	err := f.Rebuild(t.Context(), &hookLoader{values: []string{"x"}, count: 1, hookAt: -1})
	require.ErrorIs(t, err, errBusy)
	require.True(t, f.LastRebuild().IsZero())
}

// plainStorage hides the memory storage's Stats, so the filter reports only
// what it knows itself.
type plainStorage struct {
	storages.Storage
}

func TestFilter_StatsWithoutProvider(t *testing.T) {
	t.Parallel()
	f := cuckoo.New(plainStorage{Storage: memory.New()})
	st, err := f.Stats(t.Context())
	require.NoError(t, err)
	require.True(t, st.LastRebuild.IsZero())
}

var errStats = errors.New("stats unavailable")

type failingStatsStorage struct {
	storages.Storage
}

func (failingStatsStorage) Stats(context.Context) (*stats.FilterStats, error) { return nil, errStats }

func TestFilter_StatsPropagatesProviderError(t *testing.T) {
	t.Parallel()
	_, err := cuckoo.New(failingStatsStorage{Storage: memory.New()}).Stats(t.Context())
	require.ErrorIs(t, err, errStats)
}
