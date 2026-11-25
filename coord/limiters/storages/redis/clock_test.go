// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/time/clock"
	"github.com/altessa-s/go-atlas/coord/limiters/storages"
	"github.com/altessa-s/go-atlas/coord/limiters/storages/redis"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"

	limiters "github.com/altessa-s/go-atlas/coord/limiters"
)

// TestProvider_WithClock: the scripts score and prune the windows by the
// injected clock, so advancing a fake clock frees slots without a real wait
// (miniredis does not expire keys without FastForward, so only the pruning by
// the clock can free them).
func TestProvider_WithClock(t *testing.T) {
	t.Parallel()
	epoch := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	// A zero-value fake clock scores requests with negative Unix
	// milliseconds; pruning from 0 never removed them, so the window stayed
	// full until the Redis TTL expired (which miniredis never does here).
	for _, tc := range []struct {
		name string
		all  bool
	}{{"ZeroTimeAllow", false}, {"ZeroTimeAllowAll", true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			client, _ := redistest.RedisClient(t)
			fake := clock.NewFake(time.Time{})
			p := redis.New(client, redis.WithClock(fake))
			allow := func() error {
				if tc.all {
					_, err := p.AllowAll(ctx, []storages.Limit{{Key: "z", Limit: 1, Period: time.Minute}})
					return err
				}
				_, err := p.Allow(ctx, "z", 1, time.Minute)
				return err
			}
			require.NoError(t, allow())
			require.ErrorIs(t, allow(), limiters.ErrLimitExceeded)
			fake.Advance(time.Minute + time.Millisecond)
			require.NoError(t, allow(), "the window must expire on a zero-based fake clock")
		})
	}

	t.Run("Allow", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		client, _ := redistest.RedisClient(t)
		fake := clock.NewFake(epoch)
		p := redis.New(client, redis.WithClock(fake))

		_, err := p.Allow(ctx, "k", 1, time.Hour)
		require.NoError(t, err)
		info, err := p.Allow(ctx, "k", 1, time.Hour)
		require.ErrorIs(t, err, limiters.ErrLimitExceeded)
		require.Equal(t, epoch.Add(time.Hour).Unix(), info.Reset)

		fake.Advance(time.Hour + time.Millisecond)
		_, err = p.Allow(ctx, "k", 1, time.Hour)
		require.NoError(t, err, "the window must expire on the fake clock")
	})

	t.Run("AllowAll", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		client, mr := redistest.RedisClient(t)
		redistest.ServeTopology(mr, "master", "0")
		fake := clock.NewFake(epoch)
		p := redis.New(client, redis.WithClock(fake))
		limits := []storages.Limit{
			{Key: "a", Limit: 1, Period: time.Hour},
			{Key: "b", Limit: 2, Period: time.Hour},
		}

		_, err := p.AllowAll(ctx, limits)
		require.NoError(t, err)
		_, err = p.AllowAll(ctx, limits)
		require.ErrorIs(t, err, limiters.ErrLimitExceeded)

		fake.Advance(time.Hour + time.Millisecond)
		_, err = p.AllowAll(ctx, limits)
		require.NoError(t, err, "the windows must expire on the fake clock")
	})
}
