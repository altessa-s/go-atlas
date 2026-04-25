// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCoordinator_Subscribe_CheckTimeoutBoundsInitialStatus(t *testing.T) {
	c := New(
		WithCheckTimeout(10*time.Millisecond),
		WithStatusCacheTTL(0),
	)

	c.RegisterService("svc", Func(func(ctx context.Context) ServingStatus {
		<-ctx.Done()
		return StatusNotServing
	}))

	ctx := t.Context()
	done := make(chan struct{})

	go func() {
		defer close(done)
		sub, err := c.Subscribe(ctx, "svc")
		if err != nil {
			t.Errorf("Subscribe err=%v", err)
			return
		}
		sub.Close()
	}()

	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		require.Fail(t, "Subscribe did not return in time; expected check timeout to bound initial status")
	}
}

func TestCoordinator_Subscribe_WithDefaultOptions_DoesNotPanic(t *testing.T) {
	c := New(
		WithCheckTimeout(50*time.Millisecond),
		WithStatusCacheTTL(0),
	)

	c.RegisterService("svc", Func(func(context.Context) ServingStatus { return StatusServing }))

	sub, err := c.Subscribe(t.Context(), "svc")
	require.NoError(t, err)
	sub.Close()
}

func TestCoordinator_StatusDegraded(t *testing.T) {
	c := New()
	c.RegisterService("svc", Func(func(context.Context) ServingStatus { return StatusDegraded }))

	status := c.CheckStatus(t.Context(), "svc")
	require.Equal(t, StatusDegraded, status)
	require.Equal(t, "DEGRADED", status.String())
}

func TestCoordinator_Close(t *testing.T) {
	c := New()
	c.RegisterService("svc", Func(func(context.Context) ServingStatus { return StatusServing }))

	sub, err := c.Subscribe(t.Context(), "svc")
	require.NoError(t, err)

	c.Close()

	// Wait a bit to ensure watcher goroutine exits
	time.Sleep(50 * time.Millisecond)

	// After close, subscription should ideally receive StatusNotServing or be closed.
	// In our implementation, Close() calls BroadcastStatus(StatusNotServing).
	select {
	case s := <-sub.Updates():
		require.Equal(t, StatusNotServing, s, "expected StatusNotServing after Close")
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "timeout waiting for status update after Close")
	}
}

func TestCoordinator_SetOverallStatus_FlipsCheckHealth(t *testing.T) {
	c := New()
	t.Cleanup(c.Close)

	c.RegisterService("svc", Func(func(context.Context) ServingStatus { return StatusServing }))

	require.Equal(t, StatusServing, c.CheckHealth(t.Context()))

	c.SetOverallStatus(StatusNotServing)
	require.Equal(t, StatusNotServing, c.OverallStatusOverride())
	require.Equal(t, StatusNotServing, c.CheckHealth(t.Context()),
		"CheckHealth must honor SetOverallStatus regardless of healthy checkers")

	// Per-service polling and aggregated CheckStatus must agree with the override.
	require.Equal(t, StatusNotServing, c.CheckServiceHealth(t.Context(), "svc"))
	require.Equal(t, StatusNotServing, c.CheckStatus(t.Context(), ""))
	require.Equal(t, StatusNotServing, c.CheckStatus(t.Context(), "svc"))

	// Unknown services must still surface as ServiceUnknown so callers can
	// distinguish a missing checker from a shutdown signal.
	require.Equal(t, StatusServiceUnknown, c.CheckServiceHealth(t.Context(), "missing"))
}

func TestCoordinator_SetOverallStatus_BypassesCache(t *testing.T) {
	// Long TTL ensures CheckStatus would otherwise return the cached Serving entry.
	c := New(WithStatusCacheTTL(time.Hour))
	t.Cleanup(c.Close)

	c.RegisterService("svc", Func(func(context.Context) ServingStatus { return StatusServing }))

	require.Equal(t, StatusServing, c.CheckStatus(t.Context(), ""), "warm cache with Serving")

	c.SetOverallStatus(StatusNotServing)
	require.Equal(t, StatusNotServing, c.CheckStatus(t.Context(), ""),
		"override must short-circuit a stale cached overall status")
}

func TestCoordinator_SetOverallStatus_ClearedByUnknown(t *testing.T) {
	c := New()
	t.Cleanup(c.Close)

	c.RegisterService("svc", Func(func(context.Context) ServingStatus { return StatusServing }))

	c.SetOverallStatus(StatusNotServing)
	require.Equal(t, StatusNotServing, c.CheckHealth(t.Context()))

	c.SetOverallStatus(StatusUnknown)
	require.Equal(t, StatusUnknown, c.OverallStatusOverride())
	require.Equal(t, StatusServing, c.CheckHealth(t.Context()),
		"clearing the override must restore checker-driven health")
}

func TestCoordinator_Close_FlipsPullAPI(t *testing.T) {
	c := New()
	c.RegisterService("svc", Func(func(context.Context) ServingStatus { return StatusServing }))

	require.Equal(t, StatusServing, c.CheckHealth(t.Context()))

	c.Close()

	require.Equal(t, StatusNotServing, c.CheckHealth(t.Context()),
		"Close must make pull-based readers see NotServing immediately")
	require.Equal(t, StatusNotServing, c.CheckStatus(t.Context(), ""))
	require.Equal(t, StatusNotServing, c.CheckServiceHealth(t.Context(), "svc"))
}

func TestCoordinator_ListStatuses_HonorsOverride(t *testing.T) {
	c := New()
	t.Cleanup(c.Close)

	c.RegisterService("a", Func(func(context.Context) ServingStatus { return StatusServing }))
	c.RegisterService("b", Func(func(context.Context) ServingStatus { return StatusServing }))

	c.SetOverallStatus(StatusNotServing)

	statuses, err := c.ListStatuses(t.Context())
	require.NoError(t, err)
	require.Len(t, statuses, 2)
	for name, s := range statuses {
		require.Equal(t, StatusNotServing, s, "service %q should reflect override", name)
	}
}
