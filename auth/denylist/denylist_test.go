// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package denylist_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist"
)

// clockAt returns a clock fixed at base plus whatever offset the returned
// setter has been advanced to, letting a test move time deterministically.
func clockAt(base time.Time) (now func() time.Time, advance func(time.Duration)) {
	cur := base
	var mu sync.Mutex
	now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return cur
	}
	advance = func(d time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		cur = cur.Add(d)
	}
	return now, advance
}

// isRevoked reports the in-memory denylist's answer, which never errors.
func isRevoked(tb testing.TB, c interface{ IsRevoked(string) (bool, error) }, key string) bool {
	tb.Helper()
	got, err := c.IsRevoked(key)
	require.NoError(tb, err)
	return got
}

func TestRevokePermanent(t *testing.T) {
	t.Parallel()
	dl := denylist.New()
	require.False(t, isRevoked(t, dl, "a"))

	dl.Revoke("a")
	require.True(t, isRevoked(t, dl, "a"))

	dl.Restore("a")
	require.False(t, isRevoked(t, dl, "a"))
}

func TestRevokeUntilFutureThenExpires(t *testing.T) {
	t.Parallel()
	base := time.Now()
	now, advance := clockAt(base)
	dl := denylist.New(denylist.WithClock(now))

	dl.RevokeUntil("a", base.Add(time.Hour))
	require.True(t, isRevoked(t, dl, "a"))

	advance(2 * time.Hour)
	require.False(t, isRevoked(t, dl, "a")) // past expiry → not revoked
}

func TestRevokeUntilPastIsNoOp(t *testing.T) {
	t.Parallel()
	base := time.Now()
	now, _ := clockAt(base)
	dl := denylist.New(denylist.WithClock(now))

	dl.RevokeUntil("a", base.Add(-time.Second))
	require.False(t, isRevoked(t, dl, "a"))
	require.Zero(t, dl.Len()) // nothing stored
}

func TestExpiredSweptOnWrite(t *testing.T) {
	t.Parallel()
	base := time.Now()
	now, advance := clockAt(base)
	dl := denylist.New(denylist.WithClock(now))

	dl.RevokeUntil("short", base.Add(time.Minute))
	require.Equal(t, 1, dl.Len())

	advance(2 * time.Minute) // "short" is now expired but still stored
	require.Equal(t, 1, dl.Len())

	dl.Revoke("perm") // a write after the sweep interval sweeps expired entries first
	require.False(t, isRevoked(t, dl, "short"))
	require.True(t, isRevoked(t, dl, "perm"))
	require.Equal(t, 1, dl.Len()) // "short" swept, only "perm" remains
}

func TestWriteSweepsAtMostOncePerInterval(t *testing.T) {
	t.Parallel()
	base := time.Now()
	now, advance := clockAt(base)
	dl := denylist.New(denylist.WithClock(now))

	dl.RevokeUntil("short", base.Add(10*time.Second)) // first write sweeps (empty set)
	advance(30 * time.Second)                         // "short" expired, interval not yet passed

	dl.Revoke("a")
	require.False(t, isRevoked(t, dl, "short")) // expiry still enforced on read
	require.Equal(t, 2, dl.Len())               // but not swept yet

	advance(time.Minute)
	dl.Revoke("b")
	require.False(t, isRevoked(t, dl, "short"))
	require.Equal(t, 2, dl.Len()) // "short" swept; "a" and "b" remain
}

func TestSweepReclaims(t *testing.T) {
	t.Parallel()
	base := time.Now()
	now, advance := clockAt(base)
	dl := denylist.New(denylist.WithClock(now))

	dl.RevokeUntil("a", base.Add(time.Minute))
	dl.RevokeUntil("b", base.Add(2*time.Hour))
	advance(time.Hour) // "a" expired, "b" still live

	require.Equal(t, 1, dl.Sweep()) // only "a" dropped
	require.Equal(t, 1, dl.Len())
	require.True(t, isRevoked(t, dl, "b"))
}

func TestCheckerSeam(t *testing.T) {
	t.Parallel()
	dl := denylist.New()
	dl.Revoke("a")

	var c denylist.Checker = dl
	require.True(t, isRevoked(t, c, "a"))
	require.False(t, isRevoked(t, c, "b"))
}

func TestConcurrentRevokeAndCheck(t *testing.T) {
	t.Parallel()
	dl := denylist.New()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() { dl.Revoke(string(rune('a' + i%26))) })
		wg.Go(func() { _, _ = dl.IsRevoked("a") })
	}
	wg.Wait()
	require.True(t, isRevoked(t, dl, "a"))
}

// TestWithClockNilIgnored pins that a nil clock keeps the default time source.
func TestWithClockNilIgnored(t *testing.T) {
	t.Parallel()
	dl := denylist.New(denylist.WithClock(nil))
	dl.RevokeUntil("a", time.Now().Add(time.Hour))
	require.True(t, isRevoked(t, dl, "a"))
}

func TestRevocationsOnlyExtend(t *testing.T) {
	t.Parallel()
	now, advance := clockAt(epoch)
	list := denylist.New(denylist.WithClock(now))
	list.RevokeUntil("long-short", epoch.Add(time.Hour))
	list.RevokeUntil("long-short", epoch.Add(time.Minute))
	list.RevokeUntil("short-long", epoch.Add(time.Minute))
	list.RevokeUntil("short-long", epoch.Add(time.Hour))
	list.RevokeUntil("bounded-permanent", epoch.Add(time.Hour))
	list.Revoke("bounded-permanent")
	list.Revoke("permanent-bounded")
	list.RevokeUntil("permanent-bounded", epoch.Add(time.Hour))
	var writes sync.WaitGroup
	writes.Go(func() { list.Revoke("concurrent") })
	list.RevokeUntil("concurrent", epoch.Add(time.Hour))
	writes.Wait()
	advance(time.Minute)
	require.True(t, isRevoked(t, list, "long-short"))
	require.True(t, isRevoked(t, list, "short-long"))
	advance(time.Hour)
	require.False(t, isRevoked(t, list, "long-short"))
	require.False(t, isRevoked(t, list, "short-long"))
	for _, key := range []string{"bounded-permanent", "permanent-bounded", "concurrent"} {
		require.True(t, isRevoked(t, list, key), key)
		list.Restore(key)
		require.False(t, isRevoked(t, list, key))
	}
}
