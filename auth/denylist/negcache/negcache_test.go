// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package negcache_test

import (
	"context"
	"errors"
	"iter"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist/negcache"
	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom"

	bloommem "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/memory"
)

var errAuth = errors.New("authoritative unavailable")

// fakeFilter is a deterministic, exact probfilter.Filter for unit tests: no
// false positives, with an injectable MightExist error.
type fakeFilter struct {
	mu       sync.Mutex
	set      map[string]struct{}
	mightErr error
}

func newFakeFilter() *fakeFilter { return &fakeFilter{set: make(map[string]struct{})} }

func (f *fakeFilter) MightExist(_ context.Context, v string) (bool, error) {
	if f.mightErr != nil {
		return false, f.mightErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.set[v]
	return ok, nil
}

func (f *fakeFilter) Add(_ context.Context, v string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.set[v] = struct{}{}
	return nil
}

func (f *fakeFilter) AddBatch(_ context.Context, vs iter.Seq[string]) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for v := range vs {
		f.set[v] = struct{}{}
	}
	return nil
}

// fakeRebuildable is a fakeFilter that also satisfies probfilter.RebuildableFilter.
type fakeRebuildable struct {
	*fakeFilter
	rebuilt []string
}

func newFakeRebuildable() *fakeRebuildable { return &fakeRebuildable{fakeFilter: newFakeFilter()} }

func (f *fakeRebuildable) Rebuild(ctx context.Context, loader probfilter.DataLoader) error {
	for v, err := range loader.StreamValues(ctx) {
		if err != nil {
			return err
		}
		f.rebuilt = append(f.rebuilt, v)
		_ = f.fakeFilter.Add(ctx, v)
	}
	return nil
}

func (f *fakeRebuildable) LastRebuild() time.Time { return time.Time{} }

// fakeAuth is an exact Authoritative store that counts lookups.
type fakeAuth struct {
	revoked map[string]struct{}
	err     error
	calls   atomic.Int64
}

func newFakeAuth(revoked ...string) *fakeAuth {
	a := &fakeAuth{revoked: make(map[string]struct{}, len(revoked))}
	for _, k := range revoked {
		a.revoked[k] = struct{}{}
	}
	return a
}

func (a *fakeAuth) IsRevoked(_ context.Context, key string) (bool, error) {
	a.calls.Add(1)
	if a.err != nil {
		return false, a.err
	}
	_, ok := a.revoked[key]
	return ok, nil
}

func loaderFor(keys ...string) probfilter.DataLoader {
	return probfilter.NewDataLoader(func() iter.Seq[string] { return slices.Values(keys) })
}

func TestIsRevoked_FilterMiss_SkipsAuthoritative(t *testing.T) {
	t.Parallel()
	auth := newFakeAuth("revoked-elsewhere")
	c := negcache.New(newFakeFilter(), auth)

	got, err := c.IsRevoked(t.Context(), "fresh")
	require.NoError(t, err)
	require.False(t, got)
	require.Zero(t, auth.calls.Load(), "definite filter miss must not consult the authoritative store")
}

func TestIsRevoked_FilterHit_Confirmed(t *testing.T) {
	t.Parallel()
	filter := newFakeFilter()
	require.NoError(t, filter.Add(t.Context(), "jti-1"))
	auth := newFakeAuth("jti-1")
	c := negcache.New(filter, auth)

	got, err := c.IsRevoked(t.Context(), "jti-1")
	require.NoError(t, err)
	require.True(t, got)
	require.Equal(t, int64(1), auth.calls.Load())
}

func TestIsRevoked_FilterFalsePositive_AuthoritativeAllows(t *testing.T) {
	t.Parallel()
	// Filter reports present, but the authoritative store knows it is not
	// revoked: the exact store wins, so no valid token is wrongly rejected.
	filter := newFakeFilter()
	require.NoError(t, filter.Add(t.Context(), "false-positive"))
	auth := newFakeAuth() // nothing actually revoked
	c := negcache.New(filter, auth)

	got, err := c.IsRevoked(t.Context(), "false-positive")
	require.NoError(t, err)
	require.False(t, got)
	require.Equal(t, int64(1), auth.calls.Load())
}

func TestIsRevoked_FilterError_FallsBackToAuthoritative(t *testing.T) {
	t.Parallel()
	filter := newFakeFilter()
	filter.mightErr = errors.New("filter down")
	auth := newFakeAuth("jti-2")
	c := negcache.New(filter, auth)

	got, err := c.IsRevoked(t.Context(), "jti-2")
	require.NoError(t, err)
	require.True(t, got, "on filter error the authoritative store must decide")
	require.Equal(t, int64(1), auth.calls.Load())
}

func TestIsRevoked_AuthoritativeError_Propagates(t *testing.T) {
	t.Parallel()
	filter := newFakeFilter()
	require.NoError(t, filter.Add(t.Context(), "jti-3"))
	auth := newFakeAuth()
	auth.err = errAuth
	c := negcache.New(filter, auth)

	_, err := c.IsRevoked(t.Context(), "jti-3")
	require.ErrorIs(t, err, errAuth)
}

func TestAdd_MakesLookupFallThrough(t *testing.T) {
	t.Parallel()
	filter := newFakeFilter()
	auth := newFakeAuth("jti-4")
	c := negcache.New(filter, auth)

	// Before Add: filter miss, fast-pathed as not revoked despite the store.
	got, err := c.IsRevoked(t.Context(), "jti-4")
	require.NoError(t, err)
	require.False(t, got)
	require.Zero(t, auth.calls.Load())

	require.NoError(t, c.Add(t.Context(), "jti-4"))

	got, err = c.IsRevoked(t.Context(), "jti-4")
	require.NoError(t, err)
	require.True(t, got)
	require.Equal(t, int64(1), auth.calls.Load())
}

func TestRebuild_NonRebuildableFilter(t *testing.T) {
	t.Parallel()
	c := negcache.New(newFakeFilter(), newFakeAuth())
	err := c.Rebuild(t.Context(), loaderFor("a", "b"))
	require.ErrorIs(t, err, negcache.ErrFilterNotRebuildable)
}

func TestRebuild_RepopulatesFilter(t *testing.T) {
	t.Parallel()
	filter := newFakeRebuildable()
	auth := newFakeAuth("x", "y")
	c := negcache.New(filter, auth)

	require.NoError(t, c.Rebuild(t.Context(), loaderFor("x", "y")))
	require.ElementsMatch(t, []string{"x", "y"}, filter.rebuilt)

	// After a rebuild the revoked keys fall through and confirm revoked.
	got, err := c.IsRevoked(t.Context(), "x")
	require.NoError(t, err)
	require.True(t, got)
}

// TestWithRealBloomFilter wires the cache to a real probfilter Bloom filter to
// prove the interfaces line up: a definite miss is fast-pathed, an added key
// falls through to the authoritative store, and Rebuild is accepted.
func TestWithRealBloomFilter(t *testing.T) {
	t.Parallel()
	filter := bloom.New(bloommem.New())
	auth := newFakeAuth("revoked-jti")
	c := negcache.New(filter, auth)

	require.NoError(t, c.Add(t.Context(), "revoked-jti"))

	got, err := c.IsRevoked(t.Context(), "revoked-jti")
	require.NoError(t, err)
	require.True(t, got, "an added, actually-revoked key must resolve to revoked")

	require.NoError(t, c.Rebuild(t.Context(), loaderFor("revoked-jti")))
}
