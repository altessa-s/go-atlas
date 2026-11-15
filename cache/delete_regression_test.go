// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache"
	"github.com/altessa-s/go-atlas/cache/storages/lru"
)

// TestDelete_OverlappingLoadDoesNotStore pins that a load which read the
// source of truth before a delete cannot write its result back after it.
func TestDelete_OverlappingLoadDoesNotStore(t *testing.T) {
	t.Parallel()

	const key = "user:1"

	tests := []struct {
		name     string
		fallback func() (string, time.Duration, error)
		wantErr  error
		remove   func(ctx context.Context, c *cache.Cache) error
	}{
		{
			name:     "delete drops value",
			fallback: func() (string, time.Duration, error) { return "stale", cache.TTLUseDefault, nil },
			remove:   func(ctx context.Context, c *cache.Cache) error { return c.Delete(ctx, key) },
		},
		{
			name:     "delete many drops value",
			fallback: func() (string, time.Duration, error) { return "stale", cache.TTLUseDefault, nil },
			remove:   func(ctx context.Context, c *cache.Cache) error { return c.DeleteMany(ctx, "other", key) },
		},
		{
			name:     "delete drops negative entry",
			fallback: func() (string, time.Duration, error) { return "", cache.TTLUseDefault, cache.ErrMissing },
			wantErr:  cache.ErrMissing,
			remove:   func(ctx context.Context, c *cache.Cache) error { return c.Delete(ctx, key) },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			storage, err := lru.New(16)
			require.NoError(t, err)
			c := cache.New(storage, cache.WithNegativeTTL(time.Minute))
			ctx := t.Context()

			started := make(chan struct{})
			release := make(chan struct{})
			type result struct {
				value string
				err   error
			}
			done := make(chan result, 1)
			go func() {
				v, loadErr := cache.GetWithFallbackT(ctx, c, key, func(context.Context) (string, time.Duration, error) {
					close(started)
					<-release
					return tc.fallback()
				})
				done <- result{value: v, err: loadErr}
			}()

			<-started
			require.NoError(t, tc.remove(ctx, c))
			close(release)

			res := <-done
			if tc.wantErr != nil {
				require.ErrorIs(t, res.err, tc.wantErr)
			} else {
				require.NoError(t, res.err)
				require.Equal(t, "stale", res.value)
			}

			calls := 0
			fresh, err := cache.GetWithFallbackT(ctx, c, key, func(context.Context) (string, time.Duration, error) {
				calls++
				return "fresh", cache.TTLUseDefault, nil
			})
			require.NoError(t, err)
			require.Equal(t, "fresh", fresh)
			require.Equal(t, 1, calls, "the overlapped load must not have cached its result")

			cached, err := cache.GetWithFallbackT(ctx, c, key, func(context.Context) (string, time.Duration, error) {
				calls++
				return "again", cache.TTLUseDefault, nil
			})
			require.NoError(t, err)
			require.Equal(t, "fresh", cached)
			require.Equal(t, 1, calls, "a load without an overlapping delete must still cache")
		})
	}
}
