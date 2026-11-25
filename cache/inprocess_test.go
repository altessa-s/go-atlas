// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache"
	"github.com/altessa-s/go-atlas/cache/storages"
)

// deadlineStorage records whether each storage call saw a context deadline.
type deadlineStorage struct {
	mu        sync.Mutex
	data      map[string][]byte
	deadlines []bool
}

func newDeadlineStorage() *deadlineStorage {
	return &deadlineStorage{data: map[string][]byte{}}
}

func (s *deadlineStorage) record(ctx context.Context) {
	_, ok := ctx.Deadline()
	s.mu.Lock()
	s.deadlines = append(s.deadlines, ok)
	s.mu.Unlock()
}

func (s *deadlineStorage) seen() []bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]bool(nil), s.deadlines...)
}

func (s *deadlineStorage) Save(ctx context.Context, key string, value []byte, _ time.Duration) error {
	s.record(ctx)
	s.mu.Lock()
	s.data[key] = value
	s.mu.Unlock()
	return nil
}

func (s *deadlineStorage) Get(ctx context.Context, key string) ([]byte, error) {
	s.record(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[key]
	if !ok {
		return nil, storages.ErrMissing
	}
	return v, nil
}

func (s *deadlineStorage) Delete(ctx context.Context, _ string) error {
	s.record(ctx)
	return nil
}

func (s *deadlineStorage) DeleteMany(ctx context.Context, _ ...string) error {
	s.record(ctx)
	return nil
}

// inProcessStorage reports itself as served from process memory.
type inProcessStorage struct{ *deadlineStorage }

func (inProcessStorage) InProcess() bool { return true }

func exerciseCache(ctx context.Context, t *testing.T, c *cache.Cache) {
	t.Helper()

	require.NoError(t, c.Save(ctx, "k", "v"))
	var got string
	require.NoError(t, c.Get(ctx, "k", &got))
	_, err := c.Exists(ctx, "k")
	require.NoError(t, err)
	require.NoError(t, c.GetWithFallback(ctx, "k", &got, func(context.Context) (any, time.Duration, error) {
		return "v", cache.TTLUseDefault, nil
	}))
	_, err = cache.GetWithFallbackT(ctx, c, "k", func(context.Context) (string, time.Duration, error) {
		return "v", cache.TTLUseDefault, nil
	})
	require.NoError(t, err)
	_, err = c.GetManyRaw(ctx, []string{"k"})
	require.NoError(t, err)
	require.NoError(t, c.Delete(ctx, "k"))
	require.NoError(t, c.DeleteMany(ctx, "k"))
}

func TestCache_DefaultTimeoutPerStorage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		inProcess    bool
		opts         []cache.Option
		wantDeadline bool
	}{
		{name: "remote storage gets the default timeout", wantDeadline: true},
		{name: "in-process storage skips it", inProcess: true, wantDeadline: false},
		{
			name:         "in-process storage with a key version keeps it",
			inProcess:    true,
			opts:         []cache.Option{cache.WithKeyVersion(func(context.Context) (uint64, error) { return 1, nil })},
			wantDeadline: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ds := newDeadlineStorage()
			var s storages.Storage = ds
			if tc.inProcess {
				s = inProcessStorage{ds}
			}
			exerciseCache(t.Context(), t, cache.New(s, tc.opts...))

			seen := ds.seen()
			require.NotEmpty(t, seen)
			for i, got := range seen {
				require.Equal(t, tc.wantDeadline, got, "storage call %d", i)
			}
		})
	}
}

func TestCache_InProcessKeepsCallerDeadline(t *testing.T) {
	t.Parallel()

	ds := newDeadlineStorage()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	exerciseCache(ctx, t, cache.New(inProcessStorage{ds}))
	for _, got := range ds.seen() {
		require.True(t, got)
	}
}
