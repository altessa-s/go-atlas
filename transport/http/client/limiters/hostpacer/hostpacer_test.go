// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package hostpacer_test

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/client/limiters/hostpacer"
)

func TestNewRejectsNonPositive(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		interval time.Duration
		burst    int
	}{
		{"zero interval", 0, 1},
		{"negative interval", -time.Second, 1},
		{"zero burst", time.Second, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := hostpacer.New(tc.interval, tc.burst)
			require.ErrorIs(t, err, hostpacer.ErrInvalidRate)
		})
	}
}

func TestAllowSpacesPerHost(t *testing.T) {
	t.Parallel()

	p, err := hostpacer.New(time.Hour, 1)
	require.NoError(t, err)
	ctx := t.Context()
	require.NoError(t, p.Allow(ctx, "a.example"))
	require.NoError(t, p.Allow(ctx, "b.example"), "another host is not held up")

	short, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	start := time.Now()
	err = p.Allow(short, "a.example")
	require.ErrorIs(t, err, context.DeadlineExceeded, "the next slot lies beyond the deadline")
	require.Less(t, time.Since(start), 500*time.Millisecond, "a slot past the deadline is refused without waiting")
}

func TestAllowWaitsForTheSlot(t *testing.T) {
	t.Parallel()

	const interval = 50 * time.Millisecond
	p, err := hostpacer.New(interval, 1)
	require.NoError(t, err)
	require.NoError(t, p.Allow(t.Context(), "h"))
	start := time.Now()
	require.NoError(t, p.Allow(t.Context(), "h"))
	require.GreaterOrEqual(t, time.Since(start), interval-5*time.Millisecond)
}

func TestAllowBurst(t *testing.T) {
	t.Parallel()

	p, err := hostpacer.New(time.Hour, 3)
	require.NoError(t, err)
	for range 3 {
		require.NoError(t, p.Allow(t.Context(), "h"))
	}
	short, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, p.Allow(short, "h"), context.DeadlineExceeded)
}

func TestAllowCanceled(t *testing.T) {
	t.Parallel()

	p, err := hostpacer.New(time.Hour, 1)
	require.NoError(t, err)
	require.NoError(t, p.Allow(t.Context(), "h"))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, p.Allow(ctx, "h"), context.Canceled)
}

// fakeClock is a settable clock for the sweep.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func TestEvictionDropsOnlyRefilledBuckets(t *testing.T) {
	t.Parallel()

	clock := &fakeClock{t: time.Now()}
	p, err := hostpacer.New(time.Hour, 1, hostpacer.WithMaxHosts(2), hostpacer.WithNow(clock.now))
	require.NoError(t, err)
	ctx := t.Context()

	require.NoError(t, p.Allow(ctx, "a"))
	require.NoError(t, p.Allow(ctx, "b"))
	require.NoError(t, p.Allow(ctx, "c"))
	require.Equal(t, 3, p.Len(), "buckets still refilling are never dropped")

	clock.advance(2 * time.Hour)
	require.NoError(t, p.Allow(ctx, "d"))
	require.Equal(t, 1, p.Len(), "refilled buckets are dropped when a new host arrives over the cap")
}

// TestEvictionKeepsReservedBuckets: a host whose slot was just taken is not
// dropped by a sweep, so its next request still waits.
func TestEvictionKeepsReservedBuckets(t *testing.T) {
	t.Parallel()

	clock := &fakeClock{t: time.Now()}
	p, err := hostpacer.New(time.Hour, 1, hostpacer.WithMaxHosts(1), hostpacer.WithNow(clock.now))
	require.NoError(t, err)

	require.NoError(t, p.Allow(t.Context(), "x"))
	require.NoError(t, p.Allow(t.Context(), "y"), "a new host over the cap sweeps")
	require.Equal(t, 2, p.Len(), "the reserved bucket of x survives the sweep")

	short, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, p.Allow(short, "x"), context.DeadlineExceeded, "x keeps its pace")
}

// TestConcurrentEvictionNeverExceedsBurst races requests to one host against
// new hosts that keep sweeping at the cap: the host is granted burst slots,
// never more.
func TestConcurrentEvictionNeverExceedsBurst(t *testing.T) {
	t.Parallel()

	clock := &fakeClock{t: time.Now()}
	p, err := hostpacer.New(time.Hour, 1, hostpacer.WithMaxHosts(1), hostpacer.WithNow(clock.now))
	require.NoError(t, err)

	var (
		wg      sync.WaitGroup
		granted atomic.Int32
	)
	for i := range 64 {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			defer cancel()
			if p.Allow(ctx, "x") == nil {
				granted.Add(1)
			}
		})
		wg.Go(func() {
			_ = p.Allow(t.Context(), "churn-"+strconv.Itoa(i))
		})
	}
	wg.Wait()
	require.Equal(t, int32(1), granted.Load(), "one slot per interval for x, whatever the sweeps")
}
