// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters/storages"
	"github.com/altessa-s/go-atlas/data/limiters/storages/redis"

	goredis "github.com/redis/go-redis/v9"
)

func setupProvider(tb testing.TB) (*redis.Provider, *miniredis.Miniredis) {
	tb.Helper()
	mr := miniredis.RunT(tb)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
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
		// Sleep to ensure unique millisecond timestamps (Lua script uses timestamp as ZADD member)
		time.Sleep(2 * time.Millisecond)
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
		time.Sleep(2 * time.Millisecond)
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
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})

	p1 := redis.New(client, redis.WithKeyPrefix("prefix1:"))
	p2 := redis.New(client, redis.WithKeyPrefix("prefix2:"))
	ctx := t.Context()

	_, err := p1.Allow(ctx, "key", 1, time.Minute)
	require.NoError(t, err)

	// p2 should have independent limit
	_, err = p2.Allow(ctx, "key", 1, time.Minute)
	require.NoError(t, err)
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
