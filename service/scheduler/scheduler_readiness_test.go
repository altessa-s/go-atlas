// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

func TestScheduler_NoReadinessProbe_DispatchesImmediately(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(50*time.Millisecond))

	ctx := t.Context()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Wait for at least one execution
	time.Sleep(300 * time.Millisecond)

	if count := execCount.Load(); count < 1 {
		t.Errorf("expected at least 1 execution without probe, got %d", count)
	}
}

func TestScheduler_ReadinessProbe_False_SkipsTick(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithReadinessProbe(func() bool { return false }),
	)

	ctx := t.Context()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	if count := execCount.Load(); count != 0 {
		t.Errorf("expected 0 executions with probe=false, got %d", count)
	}
}

func TestScheduler_ReadinessProbe_FalseToTrue_Transition(t *testing.T) {
	storage := memory.New(100)

	var ready atomic.Bool

	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithReadinessProbe(func() bool { return ready.Load() }),
	)

	ctx := t.Context()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Probe returns false — no executions
	time.Sleep(300 * time.Millisecond)
	if count := execCount.Load(); count != 0 {
		t.Fatalf("expected 0 executions before ready, got %d", count)
	}

	// Flip to ready
	ready.Store(true)

	// Wait for at least one execution
	time.Sleep(300 * time.Millisecond)
	if count := execCount.Load(); count < 1 {
		t.Errorf("expected at least 1 execution after ready, got %d", count)
	}
}

func TestScheduler_TriggerTask_NotReady_ReturnsErrNotReady(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithReadinessProbe(func() bool { return false }),
	)

	ctx := t.Context()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	err = s.TriggerTask(ctx, "trigger-test")
	if !errors.Is(err, scheduler.ErrNotReady) {
		t.Errorf("expected ErrNotReady, got %v", err)
	}
}

func TestScheduler_ReadinessProbe_RunOnStart_FiresAfterReady(t *testing.T) {
	storage := memory.New(100)

	var ready atomic.Bool

	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithReadinessProbe(func() bool { return ready.Load() }),
	)

	ctx := t.Context()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Not ready — should not fire RunOnStart
	time.Sleep(300 * time.Millisecond)
	if count := execCount.Load(); count != 0 {
		t.Fatalf("expected 0 executions before ready, got %d", count)
	}

	// Signal ready — RunOnStart task should fire on first tick
	ready.Store(true)

	time.Sleep(300 * time.Millisecond)
	if count := execCount.Load(); count < 1 {
		t.Errorf("expected RunOnStart task to fire after ready, got %d executions", count)
	}
}

func TestScheduler_IsReady_NoProbe(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage)

	if !s.IsReady() {
		t.Error("expected IsReady()=true when no probe is configured")
	}
}

func TestScheduler_IsReady_WithProbe(t *testing.T) {
	var ready atomic.Bool

	storage := memory.New(100)
	s := scheduler.New(storage,
		scheduler.WithReadinessProbe(func() bool { return ready.Load() }),
	)

	if s.IsReady() {
		t.Error("expected IsReady()=false when probe returns false")
	}

	ready.Store(true)

	if !s.IsReady() {
		t.Error("expected IsReady()=true when probe returns true")
	}
}
