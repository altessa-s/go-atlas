// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters/storages"
	"github.com/altessa-s/go-atlas/data/limiters/storages/redis"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func setupProvider(tb testing.TB) (*redis.Provider, *miniredis.Miniredis) {
	tb.Helper()
	client, mr := testhelpers.RedisClient(tb)
	return redis.New(client), mr
}

func TestNew(t *testing.T) {
	p, _ := setupProvider(t)
	require.NotNil(t, p, "New() returned nil")
}

func TestProvider_Allow_UnderLimit(t *testing.T) {
	p, _ := setupProvider(t)
	ctx := t.Context()

	info, err := p.Allow(ctx, "key1", 5, time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(4), info.Remaining)
}

func TestProvider_Allow_ExceedsLimit(t *testing.T) {
	p, _ := setupProvider(t)
	ctx := t.Context()

	for range 3 {
		_, err := p.Allow(ctx, "key1", 3, time.Minute)
		require.NoError(t, err)
	}

	info, err := p.Allow(ctx, "key1", 3, time.Minute)
	require.ErrorIs(t, err, storages.ErrLimitExceeded)
	require.Equal(t, int64(0), info.Remaining)
}

func TestProvider_Allow_DifferentKeys(t *testing.T) {
	p, _ := setupProvider(t)
	ctx := t.Context()

	_, err := p.Allow(ctx, "key1", 1, time.Minute)
	require.NoError(t, err)

	_, err = p.Allow(ctx, "key2", 1, time.Minute)
	require.NoError(t, err)
}

func TestProvider_Reset(t *testing.T) {
	p, _ := setupProvider(t)
	ctx := t.Context()

	for range 3 {
		_, _ = p.Allow(ctx, "key1", 3, time.Minute)
	}

	err := p.Reset(ctx, "key1")
	require.NoError(t, err)

	_, err = p.Allow(ctx, "key1", 3, time.Minute)
	require.NoError(t, err)
}

func TestProvider_Reset_NonExistent(t *testing.T) {
	p, _ := setupProvider(t)
	ctx := t.Context()

	err := p.Reset(ctx, "nonexistent")
	require.NoError(t, err)
}

func TestProvider_WithKeyPrefix(t *testing.T) {
	client, _ := testhelpers.RedisClient(t)

	p1 := redis.New(client, redis.WithKeyPrefix("prefix1:"))
	p2 := redis.New(client, redis.WithKeyPrefix("prefix2:"))
	ctx := t.Context()

	_, err := p1.Allow(ctx, "key", 1, time.Minute)
	require.NoError(t, err)

	// p2 should have independent limit
	_, err = p2.Allow(ctx, "key", 1, time.Minute)
	require.NoError(t, err)
}

// TestProvider_Allow_BurstSameMillisecond is a regression test for an earlier
// bug where the Lua script used the request timestamp as both the ZSET score
// and the member. ZSET members are unique, so two calls landing in the same
// millisecond collapsed into one entry and bursts could exceed the limit. The
// fix is a per-call random member; this test verifies it by firing many
// rapid-fire calls and asserting exactly `limit` succeed.
func TestProvider_Allow_BurstSameMillisecond(t *testing.T) {
	p, _ := setupProvider(t)
	ctx := t.Context()

	const limit, attempts = 5, 20

	allowed := 0
	for range attempts {
		if _, err := p.Allow(ctx, "burst-key", limit, time.Minute); err == nil {
			allowed++
		}
	}

	require.Equal(t, limit, allowed,
		"sequential rapid-fire calls must be capped at limit; same-ms collapse would let them all through")
}

// TestProvider_Allow_ConcurrentBurst exercises the limiter from many
// goroutines at once. Without the unique-member fix, parallel ZADDs in the
// same millisecond would deduplicate and the count of allowed calls would
// exceed `limit`.
func TestProvider_Allow_ConcurrentBurst(t *testing.T) {
	p, _ := setupProvider(t)
	ctx := t.Context()

	const limit, goroutines = 10, 50

	var (
		wg      sync.WaitGroup
		allowed atomic.Int64
		start   = make(chan struct{})
	)
	for range goroutines {
		wg.Go(func() {
			<-start
			if _, err := p.Allow(ctx, "concurrent-key", limit, time.Minute); err == nil {
				allowed.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()

	require.Equal(t, int64(limit), allowed.Load(),
		"concurrent calls must be capped at limit even when they land in the same millisecond")
}

func TestProvider_Allow_Panics(t *testing.T) {
	p, _ := setupProvider(t)
	ctx := t.Context()

	tests := []struct {
		name string
		fn   func()
	}{
		{"nil context", func() { p.Allow(nil, "k", 1, time.Second) }}, //nolint:staticcheck,SA1012
		{"empty key", func() { p.Allow(ctx, "", 1, time.Second) }},
		{"zero limit", func() { p.Allow(ctx, "k", 0, time.Second) }},
		{"zero period", func() { p.Allow(ctx, "k", 1, 0) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Error("expected panic")
				}
			}()
			tt.fn()
		})
	}
}
