// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package freecache

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/storages"

	backend "github.com/coocood/freecache"
)

type cacheTimer struct{ now uint32 }

func (c *cacheTimer) Now() uint32 { return c.now }

func TestSaveFractionalTTL(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		ttl     time.Duration
		seconds uint32
	}{
		{"nanosecond", time.Nanosecond, 1},
		{"half_second", time.Second / 2, 1},
		{"fractional_seconds", time.Second + time.Nanosecond, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clock := &cacheTimer{now: 100}
			p := &Storage{cache: backend.NewCacheCustomTimer(1<<20, clock)}
			require.NoError(t, p.Save(t.Context(), "key", []byte("value"), tc.ttl))
			ttl, err := p.cache.TTL([]byte("key"))
			require.NoError(t, err)
			require.Equal(t, tc.seconds, ttl)
			clock.now += tc.seconds
			_, err = p.Get(t.Context(), "key")
			require.ErrorIs(t, err, storages.ErrMissing)
		})
	}
}

func TestSaveTTLBounds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		ttl      time.Duration
		deadline uint32
	}{
		{"zero", 0, 0},
		{"negative", -time.Nanosecond, 0},
		{"minimum_duration", time.Duration(math.MinInt64), 0},
		{"ten_years", MaxTTL, 100 + uint32(MaxTTL/time.Second)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := &Storage{cache: backend.NewCacheCustomTimer(1<<20, &cacheTimer{now: 100})}
			value := []byte("value")
			require.NoError(t, p.Save(t.Context(), "key", value, tc.ttl))
			raw, deadline, err := p.cache.PeekWithExpiration([]byte("key"))
			require.NoError(t, err)
			require.Equal(t, tc.deadline, deadline)
			require.Equal(t, value, raw)
		})
	}
	p := New(WithMaxSize(1 << 20))
	for _, ttl := range []time.Duration{MaxTTL + time.Nanosecond, MaxTTL + time.Second, time.Duration(math.MaxInt64)} {
		require.ErrorIs(t, p.Save(t.Context(), "key", nil, ttl), ErrTTLOutOfRange)
	}
}

func TestTTLSecondsIntWidth(t *testing.T) {
	t.Parallel()
	t.Run("int32", testTTLSecondsWidth[int32])
	t.Run("int64", testTTLSecondsWidth[int64])
}

func testTTLSecondsWidth[T int32 | int64](t *testing.T) {
	t.Helper()
	t.Parallel()
	seconds, err := ttlSeconds[T](MaxTTL)
	require.NoError(t, err)
	require.Equal(t, T(315360000), seconds)
	for _, ttl := range []time.Duration{(1 << 31) * time.Second, (1 << 32) * time.Second, time.Duration(math.MaxInt64)} {
		_, err = ttlSeconds[T](ttl)
		require.ErrorIs(t, err, ErrTTLOutOfRange)
	}
}
