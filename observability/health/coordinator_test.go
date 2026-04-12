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
