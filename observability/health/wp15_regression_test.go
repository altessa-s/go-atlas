// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/runtime/cycle"
	"github.com/altessa-s/go-atlas/observability/health"
)

func TestSubscribeRacingCoordinatorClose(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c := health.New()
		release := make(chan struct{})
		c.RegisterService("api", health.Func(func(context.Context) health.ServingStatus {
			<-release
			return health.StatusServing
		}))
		var sub health.Subscriber
		var err error
		var workers sync.WaitGroup
		workers.Go(func() { sub, err = c.Subscribe(t.Context(), "api") })
		synctest.Wait()
		c.Close()
		close(release)
		workers.Wait()
		if err != nil {
			require.ErrorIs(t, err, health.ErrCoordinatorShutdown)
		} else {
			defer sub.Close()
			require.Equal(t, health.StatusNotServing, sub.InitialStatus())
		}
		require.Zero(t, c.GetMetrics()["total_watchers"])
	})
}

func TestCoordinatorCloseDeliversTerminalStatus(t *testing.T) {
	t.Parallel()
	c := health.New(health.WithWatcherChannelBuffer(1))
	sub, err := c.Subscribe(t.Context(), "api")
	require.NoError(t, err)
	defer sub.Close()

	c.NotifyStatusChange("api", health.StatusServing)
	c.Close()
	require.Equal(t, health.StatusNotServing, <-sub.Updates())
	select {
	case _, open := <-sub.Updates():
		require.False(t, open)
	default:
		t.Fatal("coordinator left subscription open")
	}
	require.Zero(t, c.GetMetrics()["active_watchers"])
}

func TestCoordinatorCloseRacesUpdatesAndCancellation(t *testing.T) {
	t.Parallel()
	for range 100 {
		c := health.New(health.WithWatcherChannelBuffer(1))
		ctx, cancel := context.WithCancel(t.Context())
		sub, err := c.Subscribe(ctx, "api")
		require.NoError(t, err)
		var workers sync.WaitGroup
		workers.Go(func() {
			for range 8 {
				c.NotifyStatusChange("api", health.StatusServing)
			}
		})
		workers.Go(cancel)
		workers.Go(c.Close)
		workers.Wait()
		require.Zero(t, c.GetMetrics()["active_watchers"])
		sub.Close()
	}
}

func TestCheckerReceivesConfiguredDeadline(t *testing.T) {
	t.Parallel()
	const checkTimeout = 50 * time.Millisecond
	for _, name := range []string{"list", "service"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := health.New(health.WithCheckTimeout(checkTimeout))
			defer c.Close()
			observed := make(chan time.Duration, 1)
			c.RegisterService("api", health.Func(func(ctx context.Context) health.ServingStatus {
				deadline, _ := ctx.Deadline()
				observed <- time.Until(deadline)
				// Cancel even on the old implementation so failure does not take a minute.
				if time.Until(deadline) <= checkTimeout {
					<-ctx.Done()
				}
				return health.StatusNotServing
			}))
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			if name == "list" {
				_, err := c.ListStatuses(ctx)
				require.NoError(t, err)
			} else {
				require.Equal(t, health.StatusNotServing, c.CheckServiceHealth(ctx, "api"))
			}
			remaining := <-observed
			require.Positive(t, remaining)
			require.LessOrEqual(t, remaining, checkTimeout)
		})
	}
}

func TestHealthUsesStatusCache(t *testing.T) {
	t.Parallel()
	c := health.New(health.WithStatusCacheTTL(time.Minute))
	defer c.Close()
	var calls atomic.Int32
	c.RegisterService("api", health.Func(func(context.Context) health.ServingStatus {
		calls.Add(1)
		return health.StatusServing
	}))
	for range 100 {
		require.NoError(t, c.Health(t.Context()))
	}
	require.EqualValues(t, 1, calls.Load())
}

func TestListStatusesChecksRegistrationIdentity(t *testing.T) {
	t.Parallel()
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "removed", true: "replaced"}[replace], func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				c := health.New()
				defer c.Close()
				release := make(chan struct{})
				c.RegisterService("api", health.Func(func(context.Context) health.ServingStatus {
					<-release
					return health.StatusServing
				}))
				var statuses map[string]health.ServingStatus
				var err error
				var workers sync.WaitGroup
				workers.Go(func() { statuses, err = c.ListStatuses(t.Context()) })
				synctest.Wait()
				if replace {
					c.RegisterService("api", health.Func(func(context.Context) health.ServingStatus {
						return health.StatusNotServing
					}))
				} else {
					workers.Go(func() { c.UnregisterService("api") })
					synctest.Wait()
				}
				close(release)
				workers.Wait()
				require.NoError(t, err)
				if replace {
					require.Equal(t, health.StatusNotServing, statuses["api"])
				} else {
					require.NotContains(t, statuses, "api")
				}
			})
		})
	}
}

func TestHealthCycleReturnsSharedManagedError(t *testing.T) {
	t.Parallel()
	c := health.New()
	defer c.Close()
	c.RegisterHealthCheckSchedulerFunc()
	require.ErrorIs(t, c.RunHealthCheckCycle(t.Context()), cycle.ErrManaged)
}
