// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mirror_test

import (
	"context"
	"errors"
	"iter"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist/mirror"
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

func TestNewSnapshotStartsEmpty(t *testing.T) {
	t.Parallel()

	c := mirror.New(fakeSource{keys: []string{"a"}})
	require.False(t, c.IsRevoked("a"), "snapshot must be empty until Refresh")
	require.Zero(t, c.Len())
}

func TestRefreshReflectsSource(t *testing.T) {
	t.Parallel()

	c := mirror.New(fakeSource{keys: []string{"jti-1", "jti-2"}})
	require.NoError(t, c.Refresh(t.Context()))

	require.True(t, c.IsRevoked("jti-1"))
	require.True(t, c.IsRevoked("jti-2"))
	require.False(t, c.IsRevoked("jti-3"))
	require.Equal(t, 2, c.Len())
}

func TestRefreshShrinksAsKeysExpire(t *testing.T) {
	t.Parallel()

	// A key present in one snapshot but absent from the next becomes not-revoked
	// — the store expired it, so the mirror follows.
	c := &swapSource{keys: []string{"a", "b"}}
	m := mirror.New(c)
	require.NoError(t, m.Refresh(t.Context()))
	require.True(t, m.IsRevoked("a"))

	c.set([]string{"b"})
	require.NoError(t, m.Refresh(t.Context()))
	require.False(t, m.IsRevoked("a"))
	require.True(t, m.IsRevoked("b"))
}

func TestRefreshErrorKeepsPreviousSnapshot(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("store down")
	c := &swapSource{keys: []string{"a"}}
	m := mirror.New(c)
	require.NoError(t, m.Refresh(t.Context()))
	require.True(t, m.IsRevoked("a"))

	c.setErr(sentinel)
	err := m.Refresh(t.Context())
	require.ErrorIs(t, err, sentinel)
	require.True(t, m.IsRevoked("a"), "failed refresh must not empty the denylist")
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
				_ = m.IsRevoked("a")
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
