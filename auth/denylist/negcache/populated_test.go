// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package negcache_test

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist/negcache"
	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom"

	bloommem "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/memory"
)

// syncAuth is a concurrency-safe Authoritative store that also serves as the
// rebuild source (a probfilter.DataLoader over its revoked keys).
type syncAuth struct {
	revoked sync.Map
	calls   atomic.Int64
}

func (a *syncAuth) revoke(key string) { a.revoked.Store(key, struct{}{}) }

func (a *syncAuth) IsRevoked(_ context.Context, key string) (bool, error) {
	a.calls.Add(1)
	_, ok := a.revoked.Load(key)
	return ok, nil
}

func (a *syncAuth) StreamValues(context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		a.revoked.Range(func(k, _ any) bool { return yield(k.(string), nil) })
	}
}

func (a *syncAuth) Count(context.Context) (int64, error) { return -1, nil }

func TestIsRevoked_UnpopulatedFilterDefersToAuthoritative(t *testing.T) {
	t.Parallel()
	auth := &syncAuth{}
	auth.revoke("revoked-elsewhere")
	c := negcache.New(bloom.New(bloommem.New()), auth)

	// The filter is empty: a miss must not be trusted.
	got, err := c.IsRevoked(t.Context(), "revoked-elsewhere")
	require.NoError(t, err)
	require.True(t, got, "an unpopulated filter must not fast-path a revoked key")
	require.Equal(t, int64(1), auth.calls.Load())

	// Local Adds alone do not make the filter authoritative.
	require.NoError(t, c.Add(t.Context(), "local"))
	_, err = c.IsRevoked(t.Context(), "fresh")
	require.NoError(t, err)
	require.Equal(t, int64(2), auth.calls.Load())

	require.NoError(t, c.Rebuild(t.Context(), auth))

	got, err = c.IsRevoked(t.Context(), "fresh")
	require.NoError(t, err)
	require.False(t, got)
	require.Equal(t, int64(2), auth.calls.Load(), "after a rebuild a miss is answered locally")

	got, err = c.IsRevoked(t.Context(), "revoked-elsewhere")
	require.NoError(t, err)
	require.True(t, got)
}

func TestIsRevoked_FilterRebuiltElsewhereCountsAsPopulated(t *testing.T) {
	t.Parallel()
	auth := &syncAuth{}
	auth.revoke("jti")
	filter := bloom.New(bloommem.New())
	c := negcache.New(filter, auth)

	// Rebuilt directly (e.g. by the probfilter factory scheduler), not via c.
	require.NoError(t, filter.Rebuild(t.Context(), auth))

	got, err := c.IsRevoked(t.Context(), "fresh")
	require.NoError(t, err)
	require.False(t, got)
	require.Zero(t, auth.calls.Load())
}

func TestIsRevoked_NonRebuildableFilterAlwaysDefers(t *testing.T) {
	t.Parallel()
	auth := newFakeAuth()
	c := negcache.New(newFakeFilter(), auth)

	for range 3 {
		got, err := c.IsRevoked(t.Context(), "fresh")
		require.NoError(t, err)
		require.False(t, got)
	}
	require.Equal(t, int64(3), auth.calls.Load())
	require.ErrorIs(t, c.Rebuild(t.Context(), loaderFor()), negcache.ErrFilterNotRebuildable)
}

func TestRebuild_FailureKeepsPopulatedContents(t *testing.T) {
	t.Parallel()
	auth := &syncAuth{}
	auth.revoke("jti")
	c := negcache.New(bloom.New(bloommem.New()), auth)
	require.NoError(t, c.Rebuild(t.Context(), auth))

	errLoad := errors.New("load failed")
	failing := probfilter.DataLoaderFunc(func(context.Context) iter.Seq2[string, error] {
		return func(yield func(string, error) bool) { yield("", errLoad) }
	})
	require.ErrorIs(t, c.Rebuild(t.Context(), failing), errLoad)

	got, err := c.IsRevoked(t.Context(), "jti")
	require.NoError(t, err)
	require.True(t, got, "a failed rebuild must keep the previous contents")

	calls := auth.calls.Load()
	got, err = c.IsRevoked(t.Context(), "fresh")
	require.NoError(t, err)
	require.False(t, got)
	require.Equal(t, calls, auth.calls.Load(), "the cache stays populated after a failed rebuild")
}

// TestCache_ConcurrentRevokeDuringRebuild revokes keys (store write, then
// cache.Add) while rebuilds run and checks that no revoked key is ever
// answered "not revoked".
func TestCache_ConcurrentRevokeDuringRebuild(t *testing.T) {
	t.Parallel()
	auth := &syncAuth{}
	for i := range 100 {
		auth.revoke(fmt.Sprintf("seed-%d", i))
	}
	c := negcache.New(bloom.New(bloommem.New(bloommem.WithExpectedItems(10000))), auth)
	require.NoError(t, c.Rebuild(t.Context(), auth))

	var revoked sync.Map
	var stop atomic.Bool
	var wg sync.WaitGroup

	wg.Go(func() {
		for range 20 {
			require.NoError(t, c.Rebuild(t.Context(), auth))
		}
		stop.Store(true)
	})
	for w := range 4 {
		wg.Go(func() {
			for i := 0; !stop.Load(); i++ {
				key := fmt.Sprintf("w%d-%d", w, i)
				auth.revoke(key)
				require.NoError(t, c.Add(t.Context(), key))
				revoked.Store(key, struct{}{})
			}
		})
	}
	wg.Go(func() {
		for !stop.Load() {
			revoked.Range(func(k, _ any) bool {
				got, err := c.IsRevoked(t.Context(), k.(string))
				require.NoError(t, err)
				require.True(t, got, "revoked key %q answered not revoked", k)
				return !stop.Load()
			})
		}
	})
	wg.Wait()

	revoked.Range(func(k, _ any) bool {
		got, err := c.IsRevoked(t.Context(), k.(string))
		require.NoError(t, err)
		require.True(t, got, "revoked key %q answered not revoked", k)
		return true
	})
}
