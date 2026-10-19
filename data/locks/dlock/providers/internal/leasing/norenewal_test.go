// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leasing_test

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/internal/leasing"
)

// countingStore counts the renewals and releases reaching a memStore.
type countingStore struct {
	*memStore
	renews, releases atomic.Int32
}

func (c *countingStore) Renew(ctx context.Context, key, owner string, fencing uint64) (bool, error) {
	c.renews.Add(1)
	return c.memStore.Renew(ctx, key, owner, fencing)
}

func (c *countingStore) Release(ctx context.Context, key, owner string, fencing uint64) error {
	c.releases.Add(1)
	return c.memStore.Release(ctx, key, owner, fencing)
}

func newHardEngine(tb testing.TB, s leasing.Store, ttl time.Duration) *leasing.Engine {
	tb.Helper()
	e := leasing.New(s, leasing.Config{
		Logger: slog.New(slog.DiscardHandler), TTL: ttl, Interval: ttl / 3, OperationsTimeout: time.Second, NoRenewal: true,
	})
	tb.Cleanup(func() { _ = e.Close(context.Background()) })
	return e
}

// A hard lease is never renewed and ends one TTL after it was taken: the key
// is free again, the lock reports it lost, and releasing it late cannot end
// the next holder's lease.
func TestNoRenewal_LeaseEndsAfterTTL(t *testing.T) {
	t.Parallel()
	const ttl = 300 * time.Millisecond
	s := &countingStore{memStore: newMemStore(ttl)}
	e := newHardEngine(t, s, ttl)

	lk, err := e.Lock(t.Context(), "k")
	require.NoError(t, err)
	_, err = e.Lock(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld, "the hard lease is exclusive while it runs")

	require.Eventually(t, func() bool {
		_, err := lk.GetLockInfo(t.Context())
		return err != nil
	}, 2*time.Second, 10*time.Millisecond)
	require.Zero(t, s.renews.Load(), "a hard lease is never renewed")

	next, err := e.Lock(t.Context(), "k")
	require.NoError(t, err, "the key is free once the hard lease ended")
	require.NoError(t, lk.Release(t.Context()))
	info, err := next.GetLockInfo(t.Context())
	require.NoError(t, err, "releasing the ended lease must not end the next holder's")
	require.EqualValues(t, 2, info.FencingToken)
}

// Releasing a hard lease before its end frees the key at once.
func TestNoRenewal_ReleaseBeforeEnd(t *testing.T) {
	t.Parallel()
	s := &countingStore{memStore: newMemStore(10 * time.Second)}
	e := newHardEngine(t, s, 10*time.Second)

	lk, err := e.Lock(t.Context(), "k")
	require.NoError(t, err)
	require.NoError(t, lk.Release(t.Context()))
	require.EqualValues(t, 1, s.releases.Load())
	_, err = e.Lock(t.Context(), "k")
	require.NoError(t, err)
	require.Zero(t, s.renews.Load())
}

// Ending the lock's context before the hard lease ends releases it.
func TestNoRenewal_ContextEndReleases(t *testing.T) {
	t.Parallel()
	s := &countingStore{memStore: newMemStore(10 * time.Second)}
	e := newHardEngine(t, s, 10*time.Second)

	ctx, cancel := context.WithCancel(t.Context())
	_, err := e.Lock(ctx, "k")
	require.NoError(t, err)
	cancel()
	require.Eventually(t, func() bool {
		_, err := e.GetLockInfo(t.Context(), "k")
		return err != nil
	}, 2*time.Second, 10*time.Millisecond)
	require.EqualValues(t, 1, s.releases.Load())
}

// The hard lease is measured from the moment the acquisition request was
// sent, so a slow reply shortens it rather than extending it: once the TTL
// has passed since the request, the lock is no longer held by the engine and
// Close has nothing to release, although a full TTL has not yet passed since
// Lock returned.
func TestNoRenewal_EndMeasuredFromRequest(t *testing.T) {
	t.Parallel()
	const ttl = time.Second
	s := &countingStore{memStore: newMemStore(time.Hour)} // the store would keep it far longer
	e := newHardEngine(t, s, ttl)
	e.AfterAcquire = func() { time.Sleep(ttl / 2) }

	start := time.Now()
	_, err := e.Lock(t.Context(), "k")
	require.NoError(t, err)
	time.Sleep(time.Until(start.Add(ttl + ttl/4)))
	require.NoError(t, e.Close(t.Context()))
	require.Zero(t, s.releases.Load(), "the hard lease ended one TTL after the request, before Close")
}
