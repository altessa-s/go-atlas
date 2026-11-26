// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mirror_test

import (
	"context"
	"errors"
	"iter"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist/mirror"
	"github.com/altessa-s/go-atlas/auth/jwt"
)

// fakeSource yields keys, optionally erroring at position errAt.
type fakeSource struct {
	keys  []string
	err   error
	errAt int
}

func (f fakeSource) StreamValues(_ context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for i, k := range f.keys {
			if f.err != nil && i == f.errAt {
				yield("", f.err)
				return
			}
			if !yield(k, nil) {
				return
			}
		}
	}
}

// isRevoked reports a primed cache's answer, which never errors.
func isRevoked(tb testing.TB, c *mirror.Cache, key string) bool {
	tb.Helper()
	got, err := c.IsRevoked(key)
	require.NoError(tb, err)
	return got
}

func TestUnprimedRefusesToAnswer(t *testing.T) {
	t.Parallel()

	c := mirror.New(fakeSource{keys: []string{"a"}})
	require.False(t, c.Ready())
	_, err := c.IsRevoked("a")
	require.ErrorIs(t, err, jwt.ErrRevocationUnavailable)
	require.Zero(t, c.Len())
}

func TestInsecureServeUnprimedAnswersEmpty(t *testing.T) {
	t.Parallel()

	c := mirror.New(fakeSource{keys: []string{"a"}}, mirror.WithInsecureServeUnprimed())
	got, err := c.IsRevoked("a")
	require.NoError(t, err)
	require.False(t, got)
	require.False(t, c.Ready(), "serving unprimed does not make the cache ready")
}

func TestPrimeReturnsSourceErrorAndFlipsReady(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("store down")
	src := &swapSource{keys: []string{"a"}, err: sentinel}
	c := mirror.New(src)

	require.ErrorIs(t, c.Prime(t.Context()), sentinel)
	require.False(t, c.Ready())

	src.setErr(nil)
	require.NoError(t, c.Prime(t.Context()))
	require.True(t, c.Ready())
	require.True(t, isRevoked(t, c, "a"))
}

func TestMaxStalenessRefusesOldSnapshot(t *testing.T) {
	t.Parallel()

	var now atomic.Int64
	now.Store(1_750_000_000)
	clock := func() time.Time { return time.Unix(now.Load(), 0) }
	src := &swapSource{keys: []string{"a"}}
	c := mirror.New(src, mirror.WithMaxStaleness(time.Minute), mirror.WithClock(clock))
	require.NoError(t, c.Prime(t.Context()))

	now.Add(30)
	require.True(t, c.Ready())
	require.Equal(t, 30*time.Second, c.Age())
	require.True(t, isRevoked(t, c, "a"))

	src.setErr(errors.New("store down"))
	now.Add(31)
	require.Error(t, c.Refresh(t.Context()))
	require.False(t, c.Ready())
	_, err := c.IsRevoked("a")
	require.ErrorIs(t, err, jwt.ErrRevocationUnavailable)

	src.setErr(nil)
	require.NoError(t, c.Refresh(t.Context()))
	require.True(t, c.Ready())
	require.Zero(t, c.Age())
	require.True(t, isRevoked(t, c, "a"))
}

func TestDefaultMaxStalenessRefusesOldSnapshot(t *testing.T) {
	t.Parallel()

	require.Equal(t, 5*time.Minute, mirror.DefaultMaxStaleness)

	var now atomic.Int64
	now.Store(1_750_000_000)
	clock := func() time.Time { return time.Unix(now.Load(), 0) }

	c := mirror.New(&swapSource{keys: []string{"a"}}, mirror.WithClock(clock))
	require.NoError(t, c.Prime(t.Context()))
	now.Add(int64(mirror.DefaultMaxStaleness/time.Second) + 1)
	_, err := c.IsRevoked("a")
	require.ErrorIs(t, err, jwt.ErrRevocationUnavailable)

	off := mirror.New(&swapSource{keys: []string{"a"}}, mirror.WithClock(clock), mirror.WithMaxStaleness(0))
	require.NoError(t, off.Prime(t.Context()))
	now.Add(24 * 3600)
	require.True(t, isRevoked(t, off, "a"), "an explicit zero disables the bound")
}

// A reader sees one published snapshot: while the cache holds a stale snapshot
// it refuses, and once a refresh publishes a snapshot with different membership
// every answer comes from that snapshot. An old set paired with the new
// timestamp would answer "old is revoked" without an error.
func TestSnapshotMembershipAndFreshnessComeTogether(t *testing.T) {
	t.Parallel()

	var now atomic.Int64
	now.Store(1_750_000_000)
	clock := func() time.Time { return time.Unix(now.Load(), 0) }
	src := &gatedSource{release: make(chan struct{}, 1), keys: [][]string{{"old"}, {"new"}}}
	c := mirror.New(src, mirror.WithClock(clock), mirror.WithMaxStaleness(time.Minute))
	src.release <- struct{}{} // the priming scan
	require.NoError(t, c.Prime(t.Context()))
	got, err := c.IsRevoked("old")
	require.NoError(t, err)
	require.True(t, got)

	now.Add(3600) // the "old" snapshot is now stale
	_, err = c.IsRevoked("old")
	require.ErrorIs(t, err, jwt.ErrRevocationUnavailable)

	type answer struct {
		revoked bool
		err     error
	}
	stop := make(chan struct{})
	results := make(chan []answer, 4)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			var seen []answer
			for {
				select {
				case <-stop:
					results <- seen
					return
				default:
				}
				revoked, err := c.IsRevoked("old")
				seen = append(seen, answer{revoked, err})
			}
		})
	}
	refreshed := make(chan error, 1)
	go func() { refreshed <- c.Refresh(t.Context()) }()
	src.release <- struct{}{} // publish the "new" snapshot
	require.NoError(t, <-refreshed)
	require.Eventually(t, func() bool { return c.Ready() }, time.Second, time.Millisecond)
	close(stop)
	wg.Wait()
	close(results)

	var fresh int
	for seen := range results {
		for _, a := range seen {
			if a.err == nil {
				fresh++
				require.False(t, a.revoked, "an answer paired the old membership with the new snapshot's freshness")
			} else {
				require.ErrorIs(t, a.err, jwt.ErrRevocationUnavailable)
			}
		}
	}
	got, err = c.IsRevoked("new")
	require.NoError(t, err)
	require.True(t, got)
	require.Positive(t, fresh+1)
}

// gatedSource yields the keys of its n-th scan after a token arrives on release.
type gatedSource struct {
	release chan struct{}
	keys    [][]string
	scans   atomic.Int64
}

func (s *gatedSource) StreamValues(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		select {
		case <-s.release:
		case <-ctx.Done():
			yield("", ctx.Err())
			return
		}
		for _, k := range s.keys[s.scans.Add(1)-1] {
			if !yield(k, nil) {
				return
			}
		}
	}
}

// A source error is reported with its type only: its text may carry upstream
// detail, but errors.Is still reaches the cause.
func TestRefreshErrorDoesNotExposeSourceText(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("MARKER-secret-dsn")
	err := mirror.New(&swapSource{err: sentinel}).Refresh(t.Context())
	require.ErrorIs(t, err, sentinel)
	require.NotContains(t, err.Error(), "MARKER")
}

// A source whose iterator stops silently on cancellation must not publish a
// partial snapshot as a ready one.
func TestRefreshCanceledDoesNotPublish(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	src := cancelingSource{cancel: cancel}
	c := mirror.New(src)
	require.ErrorIs(t, c.Refresh(ctx), context.Canceled)
	require.False(t, c.Ready())
}

type cancelingSource struct{ cancel context.CancelFunc }

func (s cancelingSource) StreamValues(context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		yield("partial", nil)
		s.cancel()
	}
}

// The snapshot is stamped when the scan starts, not when it ends.
func TestRefreshStampsTimeBeforeScan(t *testing.T) {
	t.Parallel()

	var now atomic.Int64
	now.Store(1_750_000_000)
	clock := func() time.Time { return time.Unix(now.Load(), 0) }
	c := mirror.New(advancingSource{now: &now}, mirror.WithClock(clock))
	require.NoError(t, c.Prime(t.Context()))
	require.Equal(t, 100*time.Second, c.Age(), "the scan took 100s; the data is as old as the scan's start")
}

type advancingSource struct{ now *atomic.Int64 }

func (s advancingSource) StreamValues(context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		s.now.Add(100)
		yield("k", nil)
	}
}

func TestRefreshReflectsSource(t *testing.T) {
	t.Parallel()

	c := mirror.New(fakeSource{keys: []string{"jti-1", "jti-2"}})
	require.NoError(t, c.Refresh(t.Context()))

	require.True(t, isRevoked(t, c, "jti-1"))
	require.True(t, isRevoked(t, c, "jti-2"))
	require.False(t, isRevoked(t, c, "jti-3"))
	require.Equal(t, 2, c.Len())
}

func TestRefreshShrinksAsKeysExpire(t *testing.T) {
	t.Parallel()

	// A key present in one snapshot but absent from the next becomes not-revoked
	// — the store expired it, so the mirror follows.
	c := &swapSource{keys: []string{"a", "b"}}
	m := mirror.New(c)
	require.NoError(t, m.Refresh(t.Context()))
	require.True(t, isRevoked(t, m, "a"))

	c.set([]string{"b"})
	require.NoError(t, m.Refresh(t.Context()))
	require.False(t, isRevoked(t, m, "a"))
	require.True(t, isRevoked(t, m, "b"))
}

func TestRefreshErrorKeepsPreviousSnapshot(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("store down")
	c := &swapSource{keys: []string{"a"}}
	m := mirror.New(c)
	require.NoError(t, m.Refresh(t.Context()))
	require.True(t, isRevoked(t, m, "a"))

	c.setErr(sentinel)
	err := m.Refresh(t.Context())
	require.ErrorIs(t, err, sentinel)
	require.True(t, isRevoked(t, m, "a"), "failed refresh must not empty the denylist")
}

func TestConcurrentRefreshAndLookup(t *testing.T) {
	t.Parallel()

	c := &swapSource{keys: []string{"a", "b", "c"}}
	m := mirror.New(c)
	require.NoError(t, m.Refresh(t.Context()))

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 1000 {
				_, _ = m.IsRevoked("a")
			}
		})
	}
	wg.Go(func() {
		for range 100 {
			_ = m.Refresh(t.Context())
		}
	})
	wg.Wait()
}

// swapSource is a fakeSource whose keys/error can be swapped between refreshes.
type swapSource struct {
	mu   sync.Mutex
	keys []string
	err  error
}

func (s *swapSource) set(keys []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys, s.err = keys, nil
}

func (s *swapSource) setErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

func (s *swapSource) StreamValues(_ context.Context) iter.Seq2[string, error] {
	s.mu.Lock()
	keys, err := s.keys, s.err
	s.mu.Unlock()
	return func(yield func(string, error) bool) {
		if err != nil {
			yield("", err)
			return
		}
		for _, k := range keys {
			if !yield(k, nil) {
				return
			}
		}
	}
}

// pausedSource holds the older stream open while a second refresh starts.
type pausedSource struct {
	calls  atomic.Int32
	resume chan struct{}
}

func (s *pausedSource) StreamValues(context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		if s.calls.Add(1) == 1 {
			<-s.resume
			yield("old", nil)
			return
		}
		yield("new", nil)
	}
}

func TestConcurrentRefreshKeepsNewerSnapshot(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		src := &pausedSource{resume: make(chan struct{})}
		cache := mirror.New(src, mirror.WithInsecureServeUnprimed())
		done := make(chan error, 2)
		go func() { done <- cache.Refresh(t.Context()) }()
		synctest.Wait()
		go func() { done <- cache.Refresh(t.Context()) }()
		synctest.Wait()
		// Lookups must make progress while the older source read is paused.
		got, err := cache.IsRevoked("old")
		require.NoError(t, err)
		require.False(t, got)
		close(src.resume)
		require.NoError(t, <-done)
		require.NoError(t, <-done)
		require.True(t, isRevoked(t, cache, "new"), "older refresh replaced newer snapshot")
		require.False(t, isRevoked(t, cache, "old"))
		require.Equal(t, 1, cache.Len())
	})
}
