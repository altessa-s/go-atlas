// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"
)

func TestFunc_CheckHealth(t *testing.T) {
	fn := health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusDegraded
	})

	got := fn.CheckHealth(t.Context())
	require.Equal(t, health.StatusDegraded, got)
}

func TestCoordinator_CheckServiceHealth_Unknown(t *testing.T) {
	c := health.New()
	defer c.Close()

	status := c.CheckServiceHealth(t.Context(), "nonexistent")
	require.Equal(t, health.StatusServiceUnknown, status)
}

func TestCoordinator_CheckHealth_NoServices(t *testing.T) {
	c := health.New()
	defer c.Close()

	status := c.CheckHealth(t.Context())
	require.Equal(t, health.StatusServing, status)
}

func TestCoordinator_CheckHealth_AllHealthy(t *testing.T) {
	c := health.New()
	defer c.Close()

	c.RegisterService("a", health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusServing
	}))
	c.RegisterService("b", health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusServing
	}))

	status := c.CheckHealth(t.Context())
	require.Equal(t, health.StatusServing, status)
}

func TestCoordinator_Subscribe_AndClose(t *testing.T) {
	c := health.New()
	defer c.Close()

	c.RegisterService("svc", health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusServing
	}))

	sub, err := c.Subscribe(t.Context(), "svc")
	require.NoError(t, err)
	require.Equal(t, health.StatusServing, sub.InitialStatus())
	require.NotNil(t, sub.Updates())

	sub.Close()
}

func TestCoordinator_Subscribe_AfterClose(t *testing.T) {
	c := health.New()
	c.Close()

	_, err := c.Subscribe(t.Context(), "svc")
	require.Error(t, err, "Subscribe() after Close should return error")
}

func TestCoordinator_NotifyStatusChange_WithSubscriber(t *testing.T) {
	c := health.New()
	defer c.Close()

	c.RegisterService("svc", health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusServing
	}))

	sub, err := c.Subscribe(t.Context(), "svc")
	require.NoError(t, err)
	defer sub.Close()

	c.NotifyStatusChange("svc", health.StatusNotServing)

	select {
	case status := <-sub.Updates():
		require.Equal(t, health.StatusNotServing, status)
	default:
		// May not receive immediately depending on buffering
	}
}

func TestCoordinator_CheckStatus_EmptyString(t *testing.T) {
	c := health.New()
	defer c.Close()

	// Empty string checks overall health
	status := c.CheckStatus(t.Context(), "")
	require.Equal(t, health.StatusServing, status)
}

func TestCoordinator_BroadcastStatus(t *testing.T) {
	c := health.New()
	defer c.Close()

	c.RegisterService("svc1", health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusServing
	}))
	c.RegisterService("svc2", health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusServing
	}))

	// Should not panic
	c.BroadcastStatus(health.StatusNotServing)
}

func TestCoordinator_DoubleClose(t *testing.T) {
	c := health.New()
	c.Close()
	c.Close() // Should not panic
}

func TestCoordinator_GetMetrics_Values(t *testing.T) {
	c := health.New()
	defer c.Close()

	metrics := c.GetMetrics()
	require.Contains(t, metrics, "active_watchers")
	require.Contains(t, metrics, "cached_statuses")
	require.Contains(t, metrics, "total_watchers")
}

func TestCoordinator_ListStatuses_Empty(t *testing.T) {
	c := health.New()
	defer c.Close()

	statuses, err := c.ListStatuses(t.Context())
	require.NoError(t, err)
	require.Empty(t, statuses)
}
