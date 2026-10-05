// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package negcache_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist/negcache"
)

// sharedFilter is a fakeRebuildable shared with other processes: it never
// rebuilds itself (LastRebuild stays zero) but reports the shared rebuild
// state it is given.
type sharedFilter struct {
	*fakeRebuildable
	committed atomic.Bool
	err       error
	checks    atomic.Int64
	closed    atomic.Bool
}

func newSharedFilter() *sharedFilter { return &sharedFilter{fakeRebuildable: newFakeRebuildable()} }

func (f *sharedFilter) RebuildCommitted(context.Context) (bool, error) {
	f.checks.Add(1)
	return f.committed.Load(), f.err
}

func (f *sharedFilter) Close(context.Context) error {
	f.closed.Store(true)
	return nil
}

func lookup(t *testing.T, c *negcache.Cache, key string) bool {
	t.Helper()
	got, err := c.IsRevoked(t.Context(), key)
	require.NoError(t, err)
	return got
}

func TestIsRevoked_SharedRebuildCommittedElsewhere(t *testing.T) {
	t.Parallel()
	auth := newFakeAuth("revoked")
	filter := newSharedFilter()
	require.NoError(t, filter.Add(t.Context(), "revoked"))
	c := negcache.New(filter, auth, negcache.WithSharedRebuildCheckInterval(0))

	// No process has committed a rebuild: every lookup is authoritative.
	require.False(t, lookup(t, c, "fresh"))
	require.Equal(t, int64(1), auth.calls.Load())

	// Another process committed a rebuild of the shared filter.
	filter.committed.Store(true)
	require.False(t, lookup(t, c, "fresh"))
	require.Equal(t, int64(1), auth.calls.Load(), "a committed shared rebuild makes a miss trusted")
	require.True(t, lookup(t, c, "revoked"), "a key in the filter is still confirmed")

	// Populated latches: later state changes are not re-checked.
	checks := filter.checks.Load()
	filter.committed.Store(false)
	require.False(t, lookup(t, c, "fresh"))
	require.Equal(t, checks, filter.checks.Load())
}

func TestIsRevoked_SharedRebuildCheckErrorDefers(t *testing.T) {
	t.Parallel()
	auth := newFakeAuth()
	filter := newSharedFilter()
	filter.committed.Store(true)
	filter.err = errors.New("redis down")
	c := negcache.New(filter, auth, negcache.WithSharedRebuildCheckInterval(time.Nanosecond))

	require.False(t, lookup(t, c, "fresh"))
	require.Equal(t, int64(1), auth.calls.Load(), "an unknown shared state is not populated")
}

func TestIsRevoked_SharedRebuildCheckThrottled(t *testing.T) {
	t.Parallel()
	auth := newFakeAuth()
	filter := newSharedFilter()
	c := negcache.New(filter, auth, negcache.WithSharedRebuildCheckInterval(time.Hour))

	for range 10 {
		require.False(t, lookup(t, c, "fresh"))
	}
	require.Equal(t, int64(1), filter.checks.Load(), "one check per interval")
	require.Equal(t, int64(10), auth.calls.Load())
}

func TestIsRevoked_NonRebuildableNeverAsksSharedState(t *testing.T) {
	t.Parallel()
	auth := newFakeAuth()
	filter := &reportingFilter{fakeFilter: newFakeFilter()}
	c := negcache.New(filter, auth, negcache.WithSharedRebuildCheckInterval(time.Nanosecond))

	require.False(t, lookup(t, c, "fresh"))
	require.Zero(t, filter.checks.Load())
	require.Equal(t, int64(1), auth.calls.Load())
}

// reportingFilter reports a committed rebuild but cannot be rebuilt.
type reportingFilter struct {
	*fakeFilter
	checks atomic.Int64
}

func (f *reportingFilter) RebuildCommitted(context.Context) (bool, error) {
	f.checks.Add(1)
	return true, nil
}

func TestClose_ClosesFilter(t *testing.T) {
	t.Parallel()
	filter := newSharedFilter()
	c := negcache.New(filter, newFakeAuth())

	require.NoError(t, c.Close(t.Context()))
	require.True(t, filter.closed.Load())
}

func TestClose_NonCloserFilter(t *testing.T) {
	t.Parallel()
	c := negcache.New(newFakeFilter(), newFakeAuth())
	require.NoError(t, c.Close(t.Context()))
}

func TestIsRevoked_SharedRebuildCheckZeroIntervalChecksEveryLookup(t *testing.T) {
	t.Parallel()
	auth := newFakeAuth()
	filter := newSharedFilter()
	c := negcache.New(filter, auth, negcache.WithSharedRebuildCheckInterval(0))

	for range 5 {
		require.False(t, lookup(t, c, "fresh"))
	}
	require.Equal(t, int64(5), filter.checks.Load(), "a zero interval checks on every unpopulated lookup")

	filter.committed.Store(true)
	require.False(t, lookup(t, c, "fresh"))
	require.Equal(t, int64(5), auth.calls.Load(), "the very next lookup sees the committed shared rebuild")
}
