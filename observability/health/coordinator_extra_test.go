// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health_test

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"
)

func TestCoordinator_UnregisterService(t *testing.T) {
	c := health.New()
	defer c.Close()

	c.RegisterService("svc", health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusServing
	}))

	c.UnregisterService("svc")

	status := c.CheckServiceHealth(t.Context(), "svc")
	require.NotEqual(t, health.StatusServing, status, "CheckServiceHealth should not return StatusServing after unregister")
}

func TestCoordinator_ListServices(t *testing.T) {
	c := health.New()
	defer c.Close()

	c.RegisterService("svc1", health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusServing
	}))
	c.RegisterService("svc2", health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusServing
	}))

	services := make(map[string]bool)
	for name := range c.ListServices() {
		services[name] = true
	}
	require.True(t, services["svc1"], "ListServices() missing svc1")
	require.True(t, services["svc2"], "ListServices() missing svc2")
}

func TestCoordinator_ListServices_UnregisterDuringIteration(t *testing.T) {
	t.Parallel()

	// No deferred Close: on a deadlock it would block on the held lock and
	// hang the test instead of failing it.
	c := health.New()

	for _, name := range []string{"a", "b", "c"} {
		c.RegisterService(name, health.Func(func(context.Context) health.ServingStatus {
			return health.StatusServing
		}))
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for name := range c.ListServices() {
			c.UnregisterService(name)
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ListServices deadlocked when the loop body unregistered a service")
	}
	require.Empty(t, slices.Collect(c.ListServices()))
	c.Close()
}

func TestCoordinator_DroppedUpdateIsRetried(t *testing.T) {
	t.Parallel()

	var status atomic.Int32
	status.Store(int32(health.StatusServing))
	c := health.New(health.WithWatcherChannelBuffer(1))
	defer c.Close()
	cycle := func() {
		t.Helper()
		c.TriggerRecheckAll() // bypass the status cache
		require.NoError(t, c.RunHealthCheckCycle(t.Context()))
	}
	c.RegisterService("svc", health.Func(func(context.Context) health.ServingStatus {
		return health.ServingStatus(status.Load())
	}))

	sub, err := c.Subscribe(t.Context(), "svc")
	require.NoError(t, err)
	defer sub.Close()
	require.Equal(t, health.StatusServing, sub.InitialStatus())

	// Serving -> NotServing -> Serving without consuming: the buffer holds
	// NotServing and the final Serving update is dropped.
	status.Store(int32(health.StatusNotServing))
	cycle()
	status.Store(int32(health.StatusServing))
	cycle()

	select {
	case got := <-sub.Updates():
		require.Equal(t, health.StatusNotServing, got)
	default:
		t.Fatal("NotServing update was not delivered")
	}

	// The next cycle must retry the dropped Serving update.
	cycle()
	select {
	case got := <-sub.Updates():
		require.Equal(t, health.StatusServing, got)
	default:
		t.Fatal("dropped Serving update was never retried")
	}
}

func TestCoordinator_ListStatuses(t *testing.T) {
	c := health.New()
	defer c.Close()

	c.RegisterService("svc", health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusServing
	}))

	statuses, err := c.ListStatuses(t.Context())
	require.NoError(t, err)
	require.Equal(t, health.StatusServing, statuses["svc"])
}

func TestCoordinator_NotifyStatusChange(t *testing.T) {
	c := health.New()
	defer c.Close()

	c.RegisterService("svc", health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusServing
	}))

	// Should not panic
	c.NotifyStatusChange("svc", health.StatusNotServing)
}

func TestCoordinator_TriggerRecheckAll(t *testing.T) {
	c := health.New()
	defer c.Close()

	c.RegisterService("svc", health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusServing
	}))

	// Should not panic
	c.TriggerRecheckAll()
}

func TestCoordinator_GetMetrics(t *testing.T) {
	c := health.New()
	defer c.Close()

	c.RegisterService("svc", health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusServing
	}))

	metrics := c.GetMetrics()
	require.NotNil(t, metrics)
}

func TestCoordinator_CheckHealth_Aggregated(t *testing.T) {
	c := health.New()
	defer c.Close()

	c.RegisterService("healthy", health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusServing
	}))
	c.RegisterService("unhealthy", health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusNotServing
	}))

	status := c.CheckHealth(t.Context())
	// With one unhealthy service, aggregate should not be StatusServing
	require.NotEqual(t, health.StatusServing, status, "CheckHealth() should not be StatusServing when a service is unhealthy")
}
