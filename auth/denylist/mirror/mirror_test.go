// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mirror_test

import (
	"context"
	"errors"
	"iter"
	"strconv"
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

// A primed cache keeps answering while refreshes swap snapshots: every lookup
// sees one complete snapshot (never the empty seed, never an unready state).
func TestPrimedCacheAnswersDuringRefresh(t *testing.T) {
	t.Parallel()

	var tick atomic.Int64
	clock := func() time.Time { return time.Unix(tick.Load(), 0) }
	c := mirror.New(&countingSource{}, mirror.WithClock(clock), mirror.WithMaxStaleness(time.Hour))
	tick.Store(1)
	require.NoError(t, c.Prime(t.Context()))

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		defer close(done)
		for n := int64(2); n < 300; n++ {
			tick.Store(n)
			require.NoError(t, c.Refresh(t.Context()))
		}
	})
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
				}
				_, err := c.IsRevoked("k1")
				require.NoError(t, err)
				require.Equal(t, 1, c.Len())
				require.True(t, c.Ready())
			}
		})
	}
	wg.Wait()
}

type countingSource struct{ n atomic.Int64 }

func (s *countingSource) StreamValues(context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		n := s.n.Add(1)
		yield("k"+strconv.FormatInt(n, 10), nil)
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
