// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache_test

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	redisgen "github.com/altessa-s/go-atlas/data/cache/generation/storages/redis"
)

var errFill = errors.New("source down")

// source is a counting source of truth.
type source struct {
	calls atomic.Int64
	value atomic.Value // string
}

func newSource(v string) *source {
	s := &source{}
	s.value.Store(v)
	return s
}

func (s *source) fill(context.Context) (item, error) {
	s.calls.Add(1)
	v, _ := s.value.Load().(string)
	return item{Name: v, Tags: []string{"t1", "t2"}}, nil
}

func newGenerations(t testing.TB, opts ...cache.GenerationsOption) (*cache.Generations, *countingStorage) {
	t.Helper()
	storage := newLRU(t)
	return cache.NewGenerations(cache.New(storage, cache.WithFailOpen(0)), opts...), storage
}

func TestReadThrough_CachesUntilBump(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	g, _ := newGenerations(t)
	src := newSource("v1")

	for range 3 {
		got, err := cache.ReadThrough(ctx, g, "kind", "get:a", src.fill)
		require.NoError(t, err)
		require.Equal(t, "v1", got.Name)
	}
	require.EqualValues(t, 1, src.calls.Load())

	src.value.Store("v2")
	require.NoError(t, g.Bump(ctx, "kind"))

	got, err := cache.ReadThrough(ctx, g, "kind", "get:a", src.fill)
	require.NoError(t, err)
	require.Equal(t, "v2", got.Name, "a bump orphans the entry read before it")
	require.EqualValues(t, 2, src.calls.Load())
}

func TestReadThrough_NamespacesAreIndependent(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	g, _ := newGenerations(t)
	a, b, all := newSource("a"), newSource("b"), newSource("all")

	read := func(ns string, src *source) {
		t.Helper()
		_, err := cache.ReadThrough(ctx, g, ns, "k", src.fill)
		require.NoError(t, err)
	}
	read("a", a)
	read("b", b)
	read("all", all)

	require.NoError(t, g.Bump(ctx, "a", "all")) // a write to kind a also changes the cross-kind view
	read("a", a)
	read("b", b)
	read("all", all)

	require.EqualValues(t, 2, a.calls.Load())
	require.EqualValues(t, 1, b.calls.Load(), "a bump of another namespace leaves b cached")
	require.EqualValues(t, 2, all.calls.Load())
}

func TestReadThrough_KeysAreDistinctTuples(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	g, _ := newGenerations(t)

	k1, err := g.Key(ctx, "a:b", "c")
	require.NoError(t, err)
	k2, err := g.Key(ctx, "a", "b:c")
	require.NoError(t, err)
	require.NotEqual(t, k1, k2)

	src1, src2 := newSource("1"), newSource("2")
	got, err := cache.ReadThrough(ctx, g, "ns", "", src1.fill)
	require.NoError(t, err)
	require.Equal(t, "1", got.Name, "an empty key is a valid namespace-wide entry")
	got, err = cache.ReadThrough(ctx, g, "ns", "x", src2.fill)
	require.NoError(t, err)
	require.Equal(t, "2", got.Name)

	before, err := g.Key(ctx, "ns", "x")
	require.NoError(t, err)
	require.NoError(t, g.Bump(ctx, "ns"))
	after, err := g.Key(ctx, "ns", "x")
	require.NoError(t, err)
	require.NotEqual(t, before, after)
}

func TestReadThrough_FillStartedBeforeBumpIsNeverServed(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	g, _ := newGenerations(t)

	started := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	var slowErr error
	wg.Go(func() {
		_, slowErr = cache.ReadThrough(ctx, g, "ns", "k", func(context.Context) (item, error) {
			close(started)
			<-release
			return item{Name: "pre-write"}, nil
		})
	})
	<-started
	require.NoError(t, g.Bump(ctx, "ns"))
	close(release)
	wg.Wait()
	require.NoError(t, slowErr)

	src := newSource("post-write")
	got, err := cache.ReadThrough(ctx, g, "ns", "k", src.fill)
	require.NoError(t, err)
	require.Equal(t, "post-write", got.Name)
	require.EqualValues(t, 1, src.calls.Load())
}

func TestReadThrough_IsolatesReturnedValues(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	// The cache is NOT built with WithIsolatedResults: ReadThrough isolates anyway.
	g, _ := newGenerations(t)

	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce) // a failed assertion never leaks the held fill
	var fills atomic.Int64
	fill := func(context.Context) (item, error) {
		fills.Add(1)
		<-release
		return item{Name: "v", Tags: []string{"t1", "t2"}}, nil
	}

	const n = 16
	results := make([]item, n)
	errs := make([]error, n)
	var ready, wg sync.WaitGroup
	ready.Add(n)
	for i := range n {
		wg.Go(func() {
			ready.Done()
			results[i], errs[i] = cache.ReadThrough(ctx, g, "ns", "k", fill)
		})
	}
	ready.Wait()
	// The fill runs in its own goroutine, so every caller, the one that
	// started it included, parks waiting for it: hold the fill until all n
	// wait, so none can miss after the fill finished and start another.
	waitJoined(t, "TestReadThrough_IsolatesReturnedValues", n)
	releaseOnce()
	wg.Wait()

	require.EqualValues(t, 1, fills.Load(), "concurrent misses share one fill")
	for i := range n {
		require.NoError(t, errs[i])
	}
	results[0].Tags[0] = "mutated"
	for i := 1; i < n; i++ {
		require.Equal(t, "t1", results[i].Tags[0], "collapsed callers never share a slice")
	}

	hit, err := cache.ReadThrough(ctx, g, "ns", "k", fill)
	require.NoError(t, err)
	require.Equal(t, "t1", hit.Tags[0], "the cached copy is not reachable through a returned value")
	hit.Tags[1] = "mutated"
	again, err := cache.ReadThrough(ctx, g, "ns", "k", fill)
	require.NoError(t, err)
	require.Equal(t, "t2", again.Tags[1])
}

// waitJoined waits until n goroutines started by the test named test are
// parked waiting for a singleflight result: they joined the in-flight call.
// Their stacks are the observable join condition; the test name filters out
// goroutines of tests running in parallel.
func waitJoined(t testing.TB, test string, n int) {
	t.Helper()
	testhelpers.WaitFor(t, 10*time.Second, func() bool {
		buf := make([]byte, 1<<16)
		for {
			size := runtime.Stack(buf, true)
			if size < len(buf) {
				buf = buf[:size]
				break
			}
			buf = make([]byte, 2*len(buf))
		}
		joined := 0
		for stack := range strings.SplitSeq(string(buf), "\n\n") {
			header, _, _ := strings.Cut(stack, "\n")
			if strings.Contains(header, "[select") && strings.Contains(stack, "singleflight.(*Group[") &&
				strings.Contains(stack, "cache_test."+test+".") {
				joined++
			}
		}
		return joined >= n
	}, "callers did not join the in-flight fill")
}

func TestReadThrough_FreshReadsSkipTheCache(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	g, storage := newGenerations(t)
	src := newSource("v")

	fresh := cache.WithFreshReads(ctx)
	for range 2 {
		_, err := cache.ReadThrough(fresh, g, "ns", "k", src.fill)
		require.NoError(t, err)
	}
	require.EqualValues(t, 2, src.calls.Load(), "every fresh read goes to the source")
	require.Zero(t, storage.saves.Load(), "a fresh read stores nothing")

	_, err := cache.ReadThrough(ctx, g, "ns", "k", src.fill)
	require.NoError(t, err)
	_, err = cache.ReadThrough(fresh, g, "ns", "k", src.fill)
	require.NoError(t, err)
	require.EqualValues(t, 4, src.calls.Load(), "a fresh read ignores a cached entry")
}

func TestReadThrough_BypassPredicate(t *testing.T) {
	t.Parallel()
	type txKey struct{}
	g, storage := newGenerations(t, cache.WithBypass(func(ctx context.Context) bool {
		return ctx.Value(txKey{}) != nil
	}))
	src := newSource("v")
	tx := context.WithValue(t.Context(), txKey{}, true)

	for range 2 {
		_, err := cache.ReadThrough(tx, g, "ns", "k", src.fill)
		require.NoError(t, err)
	}
	require.EqualValues(t, 2, src.calls.Load())
	require.Zero(t, storage.saves.Load(), "reads in a transaction neither hit nor fill the cache")
}

func TestReadThrough_DisabledCache(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	src := newSource("v")

	disabled := cache.NewGenerations(nil)
	for range 2 {
		_, err := cache.ReadThrough(ctx, disabled, "ns", "k", src.fill)
		require.NoError(t, err)
	}
	require.NoError(t, disabled.Bump(ctx, "ns"))

	_, err := cache.ReadThrough[item](ctx, nil, "ns", "k", src.fill)
	require.NoError(t, err)
	require.EqualValues(t, 3, src.calls.Load())
}

func TestReadThrough_UnknownGenerationSkipsTheCache(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	gens := &flakyGenerations{}
	storage := newLRU(t)
	coll := testhelpers.NewTestCollector()
	g := cache.NewGenerations(
		cache.New(storage, cache.WithFailOpen(0), cache.WithCollector(coll), cache.WithName("fo")),
		cache.WithGenerationStore(gens))
	src := newSource("v")

	_, err := cache.ReadThrough(ctx, g, "ns", "k", src.fill)
	require.NoError(t, err)

	gens.down.Store(true)
	for range 2 {
		_, err = cache.ReadThrough(ctx, g, "ns", "k", src.fill)
		require.NoError(t, err)
	}
	require.EqualValues(t, 3, src.calls.Load(), "an entry of a possibly old generation is never served")
	require.EqualValues(t, 1, storage.saves.Load())
	require.InDelta(t, 2, failOpenCount(t, coll, "version"), 0)

	require.ErrorIs(t, g.Bump(ctx, "a", "b"), errGeneration)
}

// fillWithDefaultDeadline returns a fill asserting that it runs under the
// default context timeout of the cache (15 seconds) although the caller's
// context has no deadline.
func fillWithDefaultDeadline(t *testing.T) func(context.Context) (item, error) {
	return func(ctx context.Context) (item, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok, "the fill runs under the default context timeout")
		require.LessOrEqual(t, time.Until(deadline), 15*time.Second)
		return item{Name: "v"}, nil
	}
}

func nilFill(context.Context) (*item, error) { return nil, nil }

func TestReadThrough_SkippedReadsMatchTheSharedFallbacks(t *testing.T) {
	t.Parallel()
	type txKey struct{}
	paths := []struct {
		name string
		mark func(context.Context) context.Context
		down bool // the generation store fails
		// version is the fail-open "version" count of one read.
		version float64
	}{
		{name: "fresh reads", mark: cache.WithFreshReads},
		{name: "bypass", mark: func(ctx context.Context) context.Context { return context.WithValue(ctx, txKey{}, true) }},
		{name: "unknown generation", mark: func(ctx context.Context) context.Context { return ctx }, down: true, version: 1},
	}
	for _, p := range paths {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			newGens := func(opts ...cache.Option) (*cache.Generations, *countingStorage, *testhelpers.TestCollector) {
				gens := &flakyGenerations{}
				gens.down.Store(p.down)
				storage := newLRU(t)
				coll := testhelpers.NewTestCollector()
				opts = append([]cache.Option{cache.WithFailOpen(0), cache.WithCollector(coll), cache.WithName("fo")}, opts...)
				return cache.NewGenerations(cache.New(storage, opts...),
					cache.WithGenerationStore(gens),
					cache.WithBypass(func(ctx context.Context) bool { return ctx.Value(txKey{}) != nil })), storage, coll
			}
			ctx := p.mark(t.Context())

			g, storage, coll := newGens(cache.WithFallbackTimeout(time.Millisecond))
			_, err := cache.ReadThrough(ctx, g, "ns", "k", func(ctx context.Context) (item, error) {
				<-ctx.Done()
				return item{}, ctx.Err()
			})
			require.ErrorIs(t, err, context.DeadlineExceeded, "the fill runs within the fallback timeout")
			require.InDelta(t, p.version, failOpenCount(t, coll, "version"), 0, "an unknown generation is counted once per read")

			_, err = cache.ReadThrough(ctx, g, "ns", "k", nilFill)
			require.ErrorIs(t, err, cache.ErrMissing, "a nil value is ErrMissing")
			require.InDelta(t, 2*p.version, failOpenCount(t, coll, "version"), 0)
			require.Zero(t, storage.saves.Load(), "a skipped read stores nothing")

			g, _, _ = newGens(cache.WithFallbackTimeout(0))
			got, err := cache.ReadThrough(ctx, g, "ns", "k", fillWithDefaultDeadline(t))
			require.NoError(t, err)
			require.Equal(t, "v", got.Name)
		})
	}
}

func TestReadThrough_UncachedReadsKeepTheDefaults(t *testing.T) {
	t.Parallel()
	for name, g := range map[string]*cache.Generations{
		"nil generations": nil,
		"nil cache":       cache.NewGenerations(nil),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := cache.ReadThrough(t.Context(), g, "ns", "k", fillWithDefaultDeadline(t))
			require.NoError(t, err)
			require.Equal(t, "v", got.Name)

			_, err = cache.ReadThrough(t.Context(), g, "ns", "k", nilFill)
			require.ErrorIs(t, err, cache.ErrMissing, "a nil value is ErrMissing")
		})
	}
}

func TestReadThrough_FillErrorsAreNotCached(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	g, _ := newGenerations(t)

	var calls atomic.Int64
	failing := func(context.Context) (item, error) {
		calls.Add(1)
		return item{}, errFill
	}
	for range 2 {
		_, err := cache.ReadThrough(ctx, g, "ns", "k", failing)
		require.ErrorIs(t, err, errFill)
	}
	require.EqualValues(t, 2, calls.Load())
}

func TestGenerations_BumpAttemptsEveryNamespace(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	gens := &flakyGenerations{}
	g := cache.NewGenerations(nil, cache.WithGenerationStore(gens))

	require.NoError(t, g.Bump(ctx, "a", "b", "c"))
	require.EqualValues(t, 3, gens.bumps.Load())
	require.NoError(t, g.Bump(ctx))
}

func TestReadThrough_SharedStoreAcrossReplicas(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client, _ := testhelpers.RedisClient(t)
	replicaA, _ := newGenerations(t, cache.WithGenerationStore(redisgen.New(client, redisgen.WithMaxStaleness(0))))
	replicaB, _ := newGenerations(t, cache.WithGenerationStore(redisgen.New(client, redisgen.WithMaxStaleness(0))))
	src := newSource("old")

	_, err := cache.ReadThrough(ctx, replicaB, "catalog", "k", src.fill)
	require.NoError(t, err)

	src.value.Store("new")
	require.NoError(t, replicaA.Bump(ctx, "catalog"))

	got, err := cache.ReadThrough(ctx, replicaB, "catalog", "k", src.fill)
	require.NoError(t, err)
	require.Equal(t, "new", got.Name, "a write on replica A invalidates replica B")
}

func TestGetWithFallback_FreshReadsSkipTheCache(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	storage := newLRU(t)
	c := cache.New(storage)
	require.NoError(t, c.Save(ctx, "k", item{Name: "cached"}))
	saves := storage.saves.Load()

	fresh := cache.WithFreshReads(ctx)
	got, err := cache.GetWithFallbackT(fresh, c, "k", func(context.Context) (item, time.Duration, error) {
		return item{Name: "source"}, cache.TTLUseDefault, nil
	})
	require.NoError(t, err)
	require.Equal(t, "source", got.Name)

	var untyped item
	require.NoError(t, c.GetWithFallback(fresh, "k", &untyped, func(context.Context) (any, time.Duration, error) {
		return item{Name: "source"}, cache.TTLUseDefault, nil
	}))
	require.Equal(t, "source", untyped.Name)
	require.Equal(t, saves, storage.saves.Load(), "a fresh read stores nothing")

	var stored item
	require.NoError(t, c.Get(fresh, "k", &stored), "plain reads ignore the marker")
	require.Equal(t, "cached", stored.Name)
}

func TestFreshReads(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	require.False(t, cache.FreshReads(ctx))
	marked := cache.WithFreshReads(ctx)
	require.True(t, cache.FreshReads(marked))
	derived, cancel := context.WithCancel(marked)
	defer cancel()
	require.True(t, cache.FreshReads(derived), "derived contexts inherit the marker")
}
