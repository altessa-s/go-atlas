// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clock_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/retry"
	"github.com/altessa-s/go-atlas/core/time/clock"
)

var t0 = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func TestRealNow(t *testing.T) {
	t.Parallel()

	before := time.Now()
	got := clock.Real().Now()
	require.False(t, got.Before(before))
	require.False(t, got.After(time.Now()))
}

func TestRealSleep(t *testing.T) {
	t.Parallel()

	c := clock.Real()
	start := time.Now()
	require.NoError(t, c.Sleep(t.Context(), 5*time.Millisecond))
	require.GreaterOrEqual(t, time.Since(start), 5*time.Millisecond)

	require.NoError(t, c.Sleep(t.Context(), 0))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, c.Sleep(ctx, time.Hour), context.Canceled)
	require.ErrorIs(t, c.Sleep(ctx, 0), context.Canceled)

	short, stop := context.WithTimeout(t.Context(), time.Millisecond)
	defer stop()
	require.ErrorIs(t, c.Sleep(short, time.Hour), context.DeadlineExceeded)
}

func TestFake(t *testing.T) {
	t.Parallel()

	f := clock.NewFake(t0)
	require.Equal(t, t0, f.Now())

	f.Advance(time.Minute)
	require.Equal(t, t0.Add(time.Minute), f.Now())
	f.Advance(-2 * time.Minute)
	require.Equal(t, t0.Add(-time.Minute), f.Now())

	f.Set(t0)
	require.Equal(t, t0, f.Now())

	require.NoError(t, f.Sleep(t.Context(), time.Hour))
	require.Equal(t, t0.Add(time.Hour), f.Now())
	require.NoError(t, f.Sleep(t.Context(), -time.Hour))
	require.Equal(t, t0.Add(time.Hour), f.Now(), "a non-positive sleep does not move the time")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, f.Sleep(ctx, time.Second), context.Canceled)
	require.Equal(t, t0.Add(time.Hour+time.Second), f.Now())

	var zero clock.Fake
	require.True(t, zero.Now().IsZero())
}

// TestFakeDrivesRetry shows the method values fitting retry.WithSleep: the backoff runs on the fake clock instantly.
func TestFakeDrivesRetry(t *testing.T) {
	t.Parallel()

	f := clock.NewFake(t0)
	calls := 0
	err := retry.Do(t.Context(), func(context.Context) error {
		calls++
		if calls < 3 {
			return context.DeadlineExceeded
		}
		return nil
	},
		retry.WithMaxAttempts(5),
		retry.WithNextDelay(func(int, error) time.Duration { return time.Minute }),
		retry.WithSleep(f.Sleep),
	)
	require.NoError(t, err)
	require.Equal(t, 3, calls)
	require.Equal(t, t0.Add(2*time.Minute), f.Now())
}

func TestFakeConcurrent(t *testing.T) {
	t.Parallel()

	f := clock.NewFake(t0)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				f.Advance(time.Second)
				_ = f.Now()
			}
		})
	}
	wg.Wait()
	require.Equal(t, t0.Add(800*time.Second), f.Now())
}
