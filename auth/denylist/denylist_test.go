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

func TestRevokePermanent(t *testing.T) {
	t.Parallel()
	dl := denylist.New()
	require.False(t, dl.IsRevoked("a"))

	dl.Revoke("a")
	require.True(t, dl.IsRevoked("a"))

	dl.Restore("a")
	require.False(t, dl.IsRevoked("a"))
}

func TestRevokeUntilFutureThenExpires(t *testing.T) {
	t.Parallel()
	base := time.Now()
	now, advance := clockAt(base)
	dl := denylist.New(denylist.WithClock(now))

	dl.RevokeUntil("a", base.Add(time.Hour))
	require.True(t, dl.IsRevoked("a"))

	advance(2 * time.Hour)
	require.False(t, dl.IsRevoked("a")) // past expiry → not revoked
}

func TestRevokeUntilPastIsNoOp(t *testing.T) {
	t.Parallel()
	base := time.Now()
	now, _ := clockAt(base)
	dl := denylist.New(denylist.WithClock(now))

	dl.RevokeUntil("a", base.Add(-time.Second))
	require.False(t, dl.IsRevoked("a"))
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

	dl.Revoke("perm") // any write sweeps expired entries first
	require.False(t, dl.IsRevoked("short"))
	require.True(t, dl.IsRevoked("perm"))
	require.Equal(t, 1, dl.Len()) // "short" swept, only "perm" remains
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
	require.True(t, dl.IsRevoked("b"))
}

func TestCheckerSeam(t *testing.T) {
	t.Parallel()
	dl := denylist.New()
	dl.Revoke("a")

	var c denylist.Checker = dl
	require.True(t, c.IsRevoked("a"))
	require.False(t, c.IsRevoked("b"))
}

func TestConcurrentRevokeAndCheck(t *testing.T) {
	t.Parallel()
	dl := denylist.New()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() { dl.Revoke(string(rune('a' + i%26))) })
		wg.Go(func() { _ = dl.IsRevoked("a") })
	}
	wg.Wait()
	require.True(t, dl.IsRevoked("a"))
}
