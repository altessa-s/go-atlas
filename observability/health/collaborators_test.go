// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/health"
)

func TestRegistryConcurrentReplacementAndRemoval(t *testing.T) {
	t.Parallel()
	c := health.New()
	t.Cleanup(c.Close)
	checker := health.Func(func(context.Context) health.ServingStatus { return health.StatusServing })
	notify, remove := c.RegisterScopedService("db", checker)
	var wg sync.WaitGroup
	wg.Go(func() {
		notify(health.StatusDegraded)
		remove()
	})
	c.RegisterService("db", checker)
	wg.Wait()

	require.Equal(t, health.StatusServing, c.CheckStatus(t.Context(), "db"))
}

func TestListServicesTakesSnapshotOnIteration(t *testing.T) {
	t.Parallel()
	c := health.New()
	t.Cleanup(c.Close)
	checker := health.Func(func(context.Context) health.ServingStatus { return health.StatusServing })
	names := c.ListServices()
	c.RegisterService("old", checker)
	require.Equal(t, []string{"old"}, slices.Collect(names))
	c.UnregisterService("old")
	c.RegisterService("new", checker)
	require.Equal(t, []string{"new"}, slices.Collect(names))
}

func TestSubscribersObserveSamePushOrder(t *testing.T) {
	t.Parallel()
	const pushes = 32
	c := health.New(health.WithWatcherChannelBuffer(2*pushes), health.WithStatusCacheTTL(time.Hour))
	t.Cleanup(c.Close)
	c.RegisterService("db", health.Func(func(context.Context) health.ServingStatus { return health.StatusServing }))
	first, err := c.Subscribe(t.Context(), "db")
	require.NoError(t, err)
	defer first.Close()
	second, err := c.Subscribe(t.Context(), "db")
	require.NoError(t, err)
	defer second.Close()

	push := func(status health.ServingStatus) {
		for range pushes {
			c.NotifyStatusChange("db", status)
		}
	}
	var wg sync.WaitGroup
	wg.Go(func() { push(health.StatusDegraded) })
	push(health.StatusNotServing)
	wg.Wait()

	require.Len(t, first.Updates(), 2*pushes)
	require.Len(t, second.Updates(), 2*pushes)
	var last health.ServingStatus
	for range 2 * pushes {
		last = <-first.Updates()
		require.Equal(t, last, <-second.Updates())
	}
	require.Equal(t, last, c.CheckStatus(t.Context(), "db"))
}

func TestCheckSchedulerRegistersOnceAndSkipsOverlap(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		registrar := &testhelpers.MockTaskRegistrar{}
		c := health.New(health.WithScheduler(registrar), health.WithCheckSchedule("@every 1m"), health.WithStatusCacheTTL(0))
		defer c.Close()

		var wg sync.WaitGroup
		var registerErr error
		wg.Go(func() { registerErr = c.RegisterHealthChecks(t.Context()) })
		err := c.RegisterHealthChecks(t.Context())
		wg.Wait()
		require.NoError(t, err)
		require.NoError(t, registerErr)
		require.Equal(t, 1, registrar.Count())

		sub, err := c.Subscribe(t.Context(), "db")
		require.NoError(t, err)
		defer sub.Close()
		release := make(chan struct{})
		calls := 0
		c.RegisterService("db", health.Func(func(context.Context) health.ServingStatus {
			calls++
			<-release
			return health.StatusServing
		}))
		task, ok := registrar.Task("health-check")
		require.True(t, ok)
		var cycleErr error
		wg.Go(func() { cycleErr = task.Func(t.Context()) })
		synctest.Wait()
		started := calls
		overlap := make(chan error, 1)
		wg.Go(func() { overlap <- task.Func(t.Context()) })
		synctest.Wait()
		// A second tick must finish while the first probe is still blocked.
		skipped := len(overlap) == 1
		close(release)
		wg.Wait()
		require.Equal(t, 1, started)
		require.True(t, skipped)
		require.NoError(t, <-overlap)
		require.NoError(t, cycleErr)
		require.Equal(t, 1, calls)
	})
}

func TestSubscriptionLimitDuringConcurrentAdmission(t *testing.T) {
	t.Parallel()
	c := health.New(health.WithMaxWatchersPerService(1))
	t.Cleanup(c.Close)
	var wg sync.WaitGroup
	var first health.Subscriber
	var firstErr error
	wg.Go(func() { first, firstErr = c.Subscribe(t.Context(), "db") })
	second, secondErr := c.Subscribe(t.Context(), "db")
	wg.Wait()
	if firstErr != nil {
		first, second = second, first
		firstErr, secondErr = secondErr, firstErr
	}
	require.NoError(t, firstErr)
	require.ErrorIs(t, secondErr, health.ErrWatcherLimitExceeded)
	require.Nil(t, second)
	require.Equal(t, 1, c.GetMetrics()["total_watchers"])

	wg.Go(func() { c.BroadcastStatus(health.StatusDegraded) })
	first.Close()
	wg.Wait()
	require.Equal(t, 0, c.GetMetrics()["total_watchers"])
	replacement, err := c.Subscribe(t.Context(), "db")
	require.NoError(t, err)
	replacement.Close()
}

func TestShutdownBroadcastSupersedesInFlightPoll(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c := health.New(health.WithStatusCacheTTL(0))
		sub, err := c.Subscribe(t.Context(), "db")
		require.NoError(t, err)
		defer sub.Close()

		release := make(chan struct{})
		c.RegisterService("db", health.Func(func(context.Context) health.ServingStatus {
			<-release
			return health.StatusServing
		}))
		var wg sync.WaitGroup
		var cycleErr error
		wg.Go(func() { cycleErr = c.RunHealthCheckCycle(t.Context()) })
		synctest.Wait()
		c.Close()
		status := c.CheckStatus(t.Context(), "db")
		close(release)
		wg.Wait()

		require.NoError(t, cycleErr)
		require.Equal(t, health.StatusNotServing, status)
		require.Len(t, sub.Updates(), 1)
		require.Equal(t, health.StatusNotServing, <-sub.Updates())
	})
}
