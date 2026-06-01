// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

func TestScheduler_NoReadinessProbe_DispatchesImmediately(t *testing.T) {
	storage := mustNewMemory(t, 100)
	s := scheduler.New(storage, scheduler.WithTickInterval(50*time.Millisecond))

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var execCount atomic.Int32

	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:         "no-probe-task",
		Schedule:   "@every 1s",
		RunOnStart: true,
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	// Wait for at least one execution
	time.Sleep(300 * time.Millisecond)

	require.GreaterOrEqual(t, execCount.Load(), int32(1), "expected at least 1 execution without probe")
}

func TestScheduler_ReadinessProbe_False_SkipsTick(t *testing.T) {
	storage := mustNewMemory(t, 100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithReadinessProbe(func() bool { return false }),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var execCount atomic.Int32

	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:         "blocked-task",
		Schedule:   "@every 1s",
		RunOnStart: true,
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	time.Sleep(300 * time.Millisecond)

	require.Equal(t, int32(0), execCount.Load(), "expected 0 executions with probe=false")
}

func TestScheduler_ReadinessProbe_FalseToTrue_Transition(t *testing.T) {
	storage := mustNewMemory(t, 100)

	var ready atomic.Bool

	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithReadinessProbe(func() bool { return ready.Load() }),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var execCount atomic.Int32

	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:         "transition-task",
		Schedule:   "@every 1s",
		RunOnStart: true,
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	// Probe returns false — no executions
	time.Sleep(300 * time.Millisecond)
	require.Equal(t, int32(0), execCount.Load(), "expected 0 executions before ready")

	// Flip to ready
	ready.Store(true)

	// Wait for at least one execution
	time.Sleep(300 * time.Millisecond)
	require.GreaterOrEqual(t, execCount.Load(), int32(1), "expected at least 1 execution after ready")
}

func TestScheduler_TriggerTask_NotReady_ReturnsErrNotReady(t *testing.T) {
	storage := mustNewMemory(t, 100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithReadinessProbe(func() bool { return false }),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:       "trigger-test",
		Schedule: "@every 1h",
		Func:     func(_ context.Context) error { return nil },
	})
	require.NoError(t, err)

	err = s.TriggerTask(ctx, "trigger-test")
	require.ErrorIs(t, err, scheduler.ErrNotReady)
}

func TestScheduler_ReadinessProbe_RunOnStart_FiresAfterReady(t *testing.T) {
	storage := mustNewMemory(t, 100)

	var ready atomic.Bool

	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithReadinessProbe(func() bool { return ready.Load() }),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var execCount atomic.Int32

	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:         "run-on-start-task",
		Schedule:   "@every 1h",
		RunOnStart: true,
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	// Not ready — should not fire RunOnStart
	time.Sleep(300 * time.Millisecond)
	require.Equal(t, int32(0), execCount.Load(), "expected 0 executions before ready")

	// Signal ready — RunOnStart task should fire on first tick
	ready.Store(true)

	time.Sleep(300 * time.Millisecond)
	require.GreaterOrEqual(t, execCount.Load(), int32(1), "expected RunOnStart task to fire after ready")
}

func TestScheduler_IsReady_NoProbe(t *testing.T) {
	storage := mustNewMemory(t, 100)
	s := scheduler.New(storage)

	require.True(t, s.IsReady(), "expected IsReady()=true when no probe is configured")
}

func TestScheduler_IsReady_WithProbe(t *testing.T) {
	var ready atomic.Bool

	storage := mustNewMemory(t, 100)
	s := scheduler.New(storage,
		scheduler.WithReadinessProbe(func() bool { return ready.Load() }),
	)

	require.False(t, s.IsReady(), "expected IsReady()=false when probe returns false")

	ready.Store(true)

	require.True(t, s.IsReady(), "expected IsReady()=true when probe returns true")
}
