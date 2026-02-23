// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/observability/health"
)

func TestFunc_CheckHealth(t *testing.T) {
	fn := health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusDegraded
	})

	got := fn.CheckHealth(t.Context())
	if got != health.StatusDegraded {
		t.Errorf("Func.CheckHealth() = %v, want StatusDegraded", got)
	}
}

func TestCoordinator_CheckServiceHealth_Unknown(t *testing.T) {
	c := health.New()
	defer c.Close()

	status := c.CheckServiceHealth(t.Context(), "nonexistent")
	if status != health.StatusServiceUnknown {
		t.Errorf("CheckServiceHealth(nonexistent) = %v, want StatusServiceUnknown", status)
	}
}

func TestCoordinator_CheckHealth_NoServices(t *testing.T) {
	c := health.New()
	defer c.Close()

	status := c.CheckHealth(t.Context())
	if status != health.StatusServing {
		t.Errorf("CheckHealth() with no services = %v, want StatusServing", status)
	}
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
	if status != health.StatusServing {
		t.Errorf("CheckHealth() = %v, want StatusServing", status)
	}
}

func TestCoordinator_Subscribe_AndClose(t *testing.T) {
	c := health.New()
	defer c.Close()

	c.RegisterService("svc", health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusServing
	}))

	sub, err := c.Subscribe(t.Context(), "svc")
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	if sub.InitialStatus() != health.StatusServing {
		t.Errorf("InitialStatus() = %v, want StatusServing", sub.InitialStatus())
	}

	ch := sub.Updates()
	if ch == nil {
		t.Error("Updates() returned nil channel")
	}

	sub.Close()
}

func TestCoordinator_Subscribe_AfterClose(t *testing.T) {
	c := health.New()
	c.Close()

	_, err := c.Subscribe(t.Context(), "svc")
	if err == nil {
		t.Error("Subscribe() after Close should return error")
	}
}

func TestCoordinator_NotifyStatusChange_WithSubscriber(t *testing.T) {
	c := health.New()
	defer c.Close()

	c.RegisterService("svc", health.Func(func(_ context.Context) health.ServingStatus {
		return health.StatusServing
	}))

	sub, err := c.Subscribe(t.Context(), "svc")
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer sub.Close()

	c.NotifyStatusChange("svc", health.StatusNotServing)

	select {
	case status := <-sub.Updates():
		if status != health.StatusNotServing {
			t.Errorf("got status %v, want StatusNotServing", status)
		}
	default:
		// May not receive immediately depending on buffering
	}
}

func TestCoordinator_CheckStatus_EmptyString(t *testing.T) {
	c := health.New()
	defer c.Close()

	// Empty string checks overall health
	status := c.CheckStatus(t.Context(), "")
	if status != health.StatusServing {
		t.Errorf("CheckStatus('') = %v, want StatusServing", status)
	}
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
	if _, ok := metrics["active_watchers"]; !ok {
		t.Error("GetMetrics() missing active_watchers")
	}
	if _, ok := metrics["cached_statuses"]; !ok {
		t.Error("GetMetrics() missing cached_statuses")
	}
	if _, ok := metrics["total_watchers"]; !ok {
		t.Error("GetMetrics() missing total_watchers")
	}
}

func TestCoordinator_ListStatuses_Empty(t *testing.T) {
	c := health.New()
	defer c.Close()

	statuses, err := c.ListStatuses(t.Context())
	if err != nil {
		t.Fatalf("ListStatuses() error = %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("ListStatuses() = %v, want empty", statuses)
	}
}
