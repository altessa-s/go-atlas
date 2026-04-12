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
