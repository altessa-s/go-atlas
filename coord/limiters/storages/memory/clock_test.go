// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/time/clock"
	"github.com/altessa-s/go-atlas/coord/limiters/storages"
	"github.com/altessa-s/go-atlas/coord/limiters/storages/memory"

	limiters "github.com/altessa-s/go-atlas/coord/limiters"
)

var clockEpoch = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// TestProvider_WithClock_WindowFollowsFakeClock: the sliding window, the
// reset time and capacity reclaim run on the injected clock, so advancing a
// fake clock frees slots without any real wait.
func TestProvider_WithClock_WindowFollowsFakeClock(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	fake := clock.NewFake(clockEpoch)
	p := memory.New(memory.WithClock(fake))

	_, err := p.Allow(ctx, "k", 1, time.Hour)
	require.NoError(t, err)
	info, err := p.Allow(ctx, "k", 1, time.Hour)
	require.ErrorIs(t, err, limiters.ErrLimitExceeded)
	require.Equal(t, clockEpoch.Add(time.Hour).Unix(), info.Reset)

	fake.Advance(time.Hour + time.Millisecond)
	_, err = p.Allow(ctx, "k", 1, time.Hour)
	require.NoError(t, err, "the window must expire on the fake clock")
}

// TestProvider_WithClock_AllowAllAndReclaim: AllowAll windows and the
// reclaim of expired windows at capacity follow the injected clock.
func TestProvider_WithClock_AllowAllAndReclaim(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	fake := clock.NewFake(clockEpoch)
	p := memory.New(memory.WithClock(fake), memory.WithMaxBuckets(2))

	_, err := storages.AllowAll(ctx, p,
		storages.Limit{Key: "a", Limit: 1, Period: time.Minute},
		storages.Limit{Key: "b", Limit: 1, Period: time.Minute},
	)
	require.NoError(t, err)
	_, err = p.Allow(ctx, "c", 1, time.Minute)
	require.ErrorIs(t, err, memory.ErrCapacityExceeded)

	fake.Advance(time.Minute + time.Millisecond)
	_, err = p.Allow(ctx, "c", 1, time.Minute)
	require.NoError(t, err, "expired windows must be reclaimed on the fake clock")
}

// TestProvider_WithClock_Cleanup: idle cleanup measures idleness on the
// injected clock.
func TestProvider_WithClock_Cleanup(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	fake := clock.NewFake(clockEpoch)
	p := memory.New(memory.WithClock(fake), memory.WithMaxIdleTime(time.Minute), memory.WithMaxBuckets(1))

	_, err := p.Allow(ctx, "a", 5, time.Hour)
	require.NoError(t, err)
	p.RunCleanup()
	_, err = p.Allow(ctx, "b", 5, time.Hour)
	require.ErrorIs(t, err, memory.ErrCapacityExceeded, "a bucket used now is not idle")

	fake.Advance(2 * time.Minute)
	p.RunCleanup()
	_, err = p.Allow(ctx, "b", 5, time.Hour)
	require.ErrorIs(t, err, memory.ErrCapacityExceeded, "idle time alone must not remove a live window")

	fake.Advance(time.Hour)
	p.RunCleanup()
	_, err = p.Allow(ctx, "b", 5, time.Hour)
	require.NoError(t, err, "the idle bucket must be removed on the fake clock")
}

func TestProvider_CleanupKeepsLiveIdleWindow(t *testing.T) {
	t.Parallel()
	fake := clock.NewFake(time.Now())
	p := memory.New(memory.WithClock(fake))
	_, err := p.Allow(t.Context(), "hourly", 1, time.Hour)
	require.NoError(t, err)
	fake.Advance(31 * time.Minute)
	p.RunCleanup()
	_, err = p.Allow(t.Context(), "hourly", 1, time.Hour)
	require.ErrorIs(t, err, limiters.ErrLimitExceeded)
}
