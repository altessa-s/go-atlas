// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache"
	"github.com/altessa-s/go-atlas/data/cache/storages"
	"github.com/altessa-s/go-atlas/data/cache/storages/lru"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

var errStorage = errors.New("storage down")

// faultStorage wraps an LRU storage and injects failures. It does not
// implement [storages.MultiGetter]; see multiFaultStorage.
type faultStorage struct {
	storages.Storage

	getErr  error
	saveErr error
	// delay blocks Get, GetMany and Save until it elapses or ctx is done.
	delay   time.Duration
	deletes atomic.Int64
}

func newFaultStorage(tb testing.TB) *faultStorage {
	tb.Helper()
	inner, err := lru.New(128)
	require.NoError(tb, err)
	return &faultStorage{Storage: inner}
}

func (s *faultStorage) wait(ctx context.Context) error {
	if s.delay <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(s.delay):
		return nil
	}
}

func (s *faultStorage) Get(ctx context.Context, key string) ([]byte, error) {
	if err := s.wait(ctx); err != nil {
		return nil, err
	}
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.Storage.Get(ctx, key)
}

func (s *faultStorage) Save(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := s.wait(ctx); err != nil {
		return err
	}
	if s.saveErr != nil {
		return s.saveErr
	}
	return s.Storage.Save(ctx, key, value, ttl)
}

func (s *faultStorage) Delete(ctx context.Context, key string) error {
	s.deletes.Add(1)
	return s.Storage.Delete(ctx, key)
}

// multiFaultStorage is a faultStorage that also implements GetMany.
type multiFaultStorage struct {
	*faultStorage

	getManyCalls atomic.Int64
}

func (s *multiFaultStorage) GetMany(ctx context.Context, keys []string) (map[string][]byte, error) {
	s.getManyCalls.Add(1)
	if err := s.wait(ctx); err != nil {
		return nil, err
	}
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.Storage.(storages.MultiGetter).GetMany(ctx, keys)
}

func failOpenCount(t *testing.T, tc *testhelpers.TestCollector, op string) float64 {
	t.Helper()
	return testhelpers.GetCounterValue(t, tc, "test_cache_fail_open_total", "cache_name", "fo", "op", op)
}

type item struct {
	Name string
	Tags []string
}

func TestFailOpen_Lookup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*testing.T, *faultStorage)
		timeout   time.Duration
		failOpen  bool
		wantErr   error
		wantOp    string
	}{
		{name: "error without fail-open", configure: func(_ *testing.T, s *faultStorage) { s.getErr = errStorage }, wantErr: errStorage},
		{name: "error with fail-open", configure: func(_ *testing.T, s *faultStorage) { s.getErr = errStorage }, failOpen: true, wantOp: "get"},
		{
			name:      "slow storage times out",
			configure: func(_ *testing.T, s *faultStorage) { s.delay = time.Minute },
			timeout:   20 * time.Millisecond, failOpen: true, wantOp: "get",
		},
		{
			name: "corrupt value with fail-open",
			configure: func(t *testing.T, s *faultStorage) {
				require.NoError(t, s.Storage.Save(t.Context(), "k", []byte("{not json"), 0))
			},
			failOpen: true, wantOp: "decode",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := newFaultStorage(t)
			tc.configure(t, s)
			coll := testhelpers.NewTestCollector()
			opts := []cache.Option{cache.WithCollector(coll), cache.WithName("fo")}
			if tc.failOpen {
				opts = append(opts, cache.WithFailOpen(tc.timeout))
			}
			c := cache.New(s, opts...)

			var calls atomic.Int64
			got, err := cache.GetWithFallbackT(t.Context(), c, "k", func(context.Context) (item, time.Duration, error) {
				calls.Add(1)
				return item{Name: "loaded"}, cache.TTLUseDefault, nil
			})
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Zero(t, calls.Load())
				return
			}
			require.NoError(t, err)
			require.Equal(t, "loaded", got.Name)
			require.EqualValues(t, 1, calls.Load())
			require.InDelta(t, 1, failOpenCount(t, coll, tc.wantOp), 0)
			if tc.wantOp == "decode" {
				require.EqualValues(t, 1, s.deletes.Load())
			}

			// Untyped Get degrades to ErrMissing the same way.
			var v item
			err = c.Get(t.Context(), "k", &v)
			if tc.wantOp == "decode" {
				require.NoError(t, err) // the fallback rewrote the corrupt entry
				return
			}
			require.ErrorIs(t, err, cache.ErrMissing)
		})
	}
}

func TestFailOpen_GetDecodeError(t *testing.T) {
	t.Parallel()

	for _, failOpen := range []bool{false, true} {
		s := newFaultStorage(t)
		require.NoError(t, s.Storage.Save(t.Context(), "k", []byte("{not json"), 0))
		var opts []cache.Option
		if failOpen {
			opts = append(opts, cache.WithFailOpen(0))
		}
		c := cache.New(s, opts...)

		var v item
		err := c.Get(t.Context(), "k", &v)
		if !failOpen {
			require.Error(t, err)
			require.NotErrorIs(t, err, cache.ErrMissing)
			require.Zero(t, s.deletes.Load())
			continue
		}
		require.ErrorIs(t, err, cache.ErrMissing)
		require.EqualValues(t, 1, s.deletes.Load())
	}
}

func TestFailOpen_UntypedCorruptValueReloads(t *testing.T) {
	t.Parallel()

	s := newFaultStorage(t)
	require.NoError(t, s.Storage.Save(t.Context(), "k", []byte(`{"Name":"stale","Tags":"bad"}`), 0))
	c := cache.New(s, cache.WithFailOpen(0))

	var v item
	err := c.GetWithFallback(t.Context(), "k", &v, func(context.Context) (any, time.Duration, error) {
		return &item{Name: "fresh"}, cache.TTLUseDefault, nil
	})
	require.NoError(t, err)
	require.Equal(t, item{Name: "fresh"}, v)
}

func TestFailOpen_Save(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		saveErr  error
		delay    time.Duration
		failOpen bool
		wantErr  bool
	}{
		{name: "error without fail-open", saveErr: errStorage, wantErr: true},
		{name: "error with fail-open", saveErr: errStorage, failOpen: true},
		{name: "timeout with fail-open", delay: time.Minute, failOpen: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := newFaultStorage(t)
			s.saveErr, s.delay = tc.saveErr, tc.delay
			coll := testhelpers.NewTestCollector()
			opts := []cache.Option{cache.WithCollector(coll), cache.WithName("fo")}
			if tc.failOpen {
				opts = append(opts, cache.WithFailOpen(20*time.Millisecond))
			}
			c := cache.New(s, opts...)

			err := c.Save(t.Context(), "k", item{Name: "x"})
			if tc.wantErr {
				require.ErrorIs(t, err, errStorage)
				return
			}
			require.NoError(t, err)
			require.InDelta(t, 1, failOpenCount(t, coll, "save"), 0)
		})
	}
}

func TestFailOpen_SaveCanceledCallerIsReported(t *testing.T) {
	t.Parallel()

	s := newFaultStorage(t)
	s.delay = time.Minute
	c := cache.New(s, cache.WithFailOpen(0))
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, c.Save(ctx, "k", "v"), context.DeadlineExceeded)
}

func TestFailOpen_SaveAfterFallback(t *testing.T) {
	t.Parallel()

	for _, failOpen := range []bool{false, true} {
		s := newFaultStorage(t)
		s.saveErr = errStorage
		coll := testhelpers.NewTestCollector()
		opts := []cache.Option{cache.WithCollector(coll), cache.WithName("fo")}
		if failOpen {
			opts = append(opts, cache.WithFailOpen(0))
		}
		c := cache.New(s, opts...)

		got, err := cache.GetWithFallbackT(t.Context(), c, "k", func(context.Context) (string, time.Duration, error) {
			return "loaded", cache.TTLUseDefault, nil
		})
		if !failOpen {
			require.ErrorIs(t, err, errStorage)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, "loaded", got)
		require.InDelta(t, 1, failOpenCount(t, coll, "save"), 0)
	}
}

func TestFailOpen_FallbackErrorsPropagate(t *testing.T) {
	t.Parallel()

	errLoad := errors.New("load failed")
	c := cache.New(newFaultStorage(t), cache.WithFailOpen(0), cache.WithNegativeTTL(time.Minute))

	_, err := cache.GetWithFallbackT(t.Context(), c, "a", func(context.Context) (string, time.Duration, error) {
		return "", 0, errLoad
	})
	require.ErrorIs(t, err, errLoad)

	_, err = cache.GetWithFallbackT(t.Context(), c, "b", func(context.Context) (string, time.Duration, error) {
		return "", 0, cache.ErrMissing
	})
	require.ErrorIs(t, err, cache.ErrMissing)
	// The negative entry answers the next lookup without the fallback.
	_, err = cache.GetWithFallbackT(t.Context(), c, "b", func(context.Context) (string, time.Duration, error) {
		panic("negative entry must be served")
	})
	require.ErrorIs(t, err, cache.ErrMissing)
}

func TestFailOpen_Overload(t *testing.T) {
	t.Parallel()

	for _, failOpen := range []bool{false, true} {
		coll := testhelpers.NewTestCollector()
		opts := []cache.Option{cache.WithCollector(coll), cache.WithName("fo"), cache.WithMaxConcurrentFallbacks(1)}
		if failOpen {
			opts = append(opts, cache.WithFailOpen(0))
		}
		c := cache.New(newFaultStorage(t), opts...)

		// Occupy the only admission slot with another key.
		entered, release := make(chan struct{}), make(chan struct{})
		var wg sync.WaitGroup
		wg.Go(func() {
			_, _ = cache.GetWithFallbackT(t.Context(), c, "busy", func(context.Context) (string, time.Duration, error) {
				close(entered)
				<-release
				return "busy", cache.TTLUseDefault, nil
			})
		})
		<-entered

		got, err := cache.GetWithFallbackT(t.Context(), c, "k", func(context.Context) (string, time.Duration, error) {
			return "direct", cache.TTLUseDefault, nil
		})
		close(release)
		wg.Wait()

		if !failOpen {
			require.ErrorIs(t, err, cache.ErrOverloaded)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, "direct", got)
		require.InDelta(t, 1, failOpenCount(t, coll, "overload"), 0)
		var stored string
		require.NoError(t, c.Get(t.Context(), "k", &stored))
		require.Equal(t, "direct", stored)
	}
}

func TestIsolatedResults(t *testing.T) {
	t.Parallel()

	const waiters = 16

	tests := []struct {
		name  string
		typed bool
	}{
		{name: "typed", typed: true},
		{name: "untyped"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := cache.New(newFaultStorage(t), cache.WithIsolatedResults())

			release := make(chan struct{})
			var calls atomic.Int64
			fallback := func(context.Context) (item, time.Duration, error) {
				calls.Add(1)
				<-release
				return item{Name: "shared", Tags: []string{"a", "b"}}, cache.TTLUseDefault, nil
			}

			results := make([]item, waiters)
			errs := make([]error, waiters)
			var started, done sync.WaitGroup
			for i := range waiters {
				started.Add(1)
				done.Go(func() {
					started.Done()
					if tc.typed {
						results[i], errs[i] = cache.GetWithFallbackT(t.Context(), c, "k", fallback)
					} else {
						errs[i] = c.GetWithFallback(t.Context(), "k", &results[i], func(ctx context.Context) (any, time.Duration, error) {
							return fallback(ctx)
						})
					}
					if errs[i] == nil {
						// Mutate the private copy; with shared results this races
						// with the other callers and corrupts what they see.
						results[i].Tags[0] = string(rune('A' + i))
					}
				})
			}
			started.Wait()
			time.Sleep(20 * time.Millisecond) // let the waiters join the flight
			close(release)
			done.Wait()

			require.EqualValues(t, 1, calls.Load())
			for i := range waiters {
				require.NoError(t, errs[i])
				require.Equal(t, []string{string(rune('A' + i)), "b"}, results[i].Tags)
			}
		})
	}
}

func TestGetManyRaw(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		multi bool
	}{
		{name: "multi getter", multi: true},
		{name: "loop of Get"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fs := newFaultStorage(t)
			var s cache.Storage = fs
			mfs := &multiFaultStorage{faultStorage: fs}
			if tc.multi {
				s = mfs
			}
			coll := testhelpers.NewTestCollector()
			c := cache.New(s, cache.WithCollector(coll), cache.WithName("fo"), cache.WithNegativeTTL(time.Minute))

			require.NoError(t, c.Save(t.Context(), "a", "va"))
			require.NoError(t, c.Save(t.Context(), "b", "vb"))
			_, err := cache.GetWithFallbackT(t.Context(), c, "neg", func(context.Context) (string, time.Duration, error) {
				return "", 0, cache.ErrMissing
			})
			require.ErrorIs(t, err, cache.ErrMissing)

			raw, err := c.GetManyRaw(t.Context(), []string{"a", "b", "a", "missing", "neg"})
			require.NoError(t, err)
			require.Equal(t, map[string][]byte{"a": []byte(`"va"`), "b": []byte(`"vb"`)}, raw)
			if tc.multi {
				require.EqualValues(t, 1, mfs.getManyCalls.Load())
			}
			require.InDelta(t, 2, testhelpers.GetCounterValue(t, coll, "test_cache_hits_total", "cache_name", "fo"), 0)
			require.InDelta(t, 1, testhelpers.GetCounterValue(t, coll, "test_cache_negative_hits_total", "cache_name", "fo"), 0)
			// "neg" missed once before its fallback ran, "missing" once here.
			require.InDelta(t, 2, testhelpers.GetCounterValue(t, coll, "test_cache_misses_total", "cache_name", "fo"), 0)

			typed, err := cache.GetManyT[string](t.Context(), c, []string{"a", "b", "missing"})
			require.NoError(t, err)
			require.Equal(t, map[string]string{"a": "va", "b": "vb"}, typed)

			empty, err := c.GetManyRaw(t.Context(), nil)
			require.NoError(t, err)
			require.Empty(t, empty)
		})
	}
}

func TestGetManyRaw_FailOpen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		multi    bool
		delay    time.Duration
		failOpen bool
	}{
		{name: "multi error", multi: true},
		{name: "loop error"},
		{name: "multi error fail-open", multi: true, failOpen: true},
		{name: "loop error fail-open", failOpen: true},
		{name: "multi timeout fail-open", multi: true, delay: time.Minute, failOpen: true},
		{name: "loop timeout fail-open", delay: time.Minute, failOpen: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fs := newFaultStorage(t)
			var s cache.Storage = fs
			if tc.multi {
				s = &multiFaultStorage{faultStorage: fs}
			}
			if tc.delay > 0 {
				fs.delay = tc.delay
			} else {
				fs.getErr = errStorage
			}
			var opts []cache.Option
			if tc.failOpen {
				opts = append(opts, cache.WithFailOpen(20*time.Millisecond))
			}
			c := cache.New(s, opts...)

			raw, err := c.GetManyRaw(t.Context(), []string{"a", "b"})
			if !tc.failOpen {
				require.ErrorIs(t, err, errStorage)
				return
			}
			require.NoError(t, err)
			require.Empty(t, raw)
		})
	}
}

func TestGetManyT_DecodeError(t *testing.T) {
	t.Parallel()

	for _, failOpen := range []bool{false, true} {
		fs := newFaultStorage(t)
		var opts []cache.Option
		if failOpen {
			opts = append(opts, cache.WithFailOpen(0))
		}
		c := cache.New(&multiFaultStorage{faultStorage: fs}, opts...)
		require.NoError(t, c.Save(t.Context(), "good", item{Name: "g", Tags: []string{"x"}}))
		require.NoError(t, fs.Storage.Save(t.Context(), "bad", []byte("{not json"), 0))

		got, err := cache.GetManyT[item](t.Context(), c, []string{"good", "bad"})
		if !failOpen {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, map[string]item{"good": {Name: "g", Tags: []string{"x"}}}, got)
		require.EqualValues(t, 1, fs.deletes.Load())
		_, err = fs.Storage.Get(t.Context(), "bad")
		require.ErrorIs(t, err, storages.ErrMissing)
	}
}

func TestGetManyRaw_Namespace(t *testing.T) {
	t.Parallel()

	fs := newFaultStorage(t)
	c := cache.New(&multiFaultStorage{faultStorage: fs},
		cache.WithKeyNamespace(func(ctx context.Context) string { return ctx.Value(namespaceKey{}).(string) }))

	ctxA := context.WithValue(t.Context(), namespaceKey{}, "tenant-a")
	ctxB := context.WithValue(t.Context(), namespaceKey{}, "tenant-b")
	require.NoError(t, c.Save(ctxA, "k", "a"))
	require.NoError(t, c.Save(ctxB, "k", "b"))
	require.NoError(t, c.Save(ctxA, "only-a", "a2"))

	gotA, err := cache.GetManyT[string](ctxA, c, []string{"k", "only-a"})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"k": "a", "only-a": "a2"}, gotA)

	rawB, err := c.GetManyRaw(ctxB, []string{"k", "only-a"})
	require.NoError(t, err)
	require.Equal(t, map[string][]byte{"k": []byte(`"b"`)}, rawB)

	// The storage holds namespaced keys only.
	_, err = fs.Storage.Get(t.Context(), "k")
	require.ErrorIs(t, err, storages.ErrMissing)
}
