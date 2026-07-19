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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/service/scheduler"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

func TestScheduler_Metrics_Noop(t *testing.T) {
	s, ctx := startScheduler(t)
	execCount := registerCountingTask(t, ctx, s, "noop-task", "@every 1s")

	time.Sleep(200 * time.Millisecond)
	assert.GreaterOrEqual(t, execCount.Load(), int32(1), "task should have executed at least once")
}

func TestScheduler_Metrics_TasksRegistered(t *testing.T) {
	tc := testhelpers.NewTestCollector()

	storage := mustNewMemory(t, 100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithCollector(tc),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	noop := func(_ context.Context) error { return nil }

	// Register two tasks
	require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
		ID: "task-a", Schedule: "@every 1h", Func: noop,
	}))
	require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
		ID: "task-b", Schedule: "@every 1h", Func: noop,
	}))

	val := testhelpers.GetGaugeValue(t, tc, "test_scheduler_tasks_registered")
	assert.Equal(t, float64(2), val, "should have 2 registered tasks")

	// Unregister one
	require.NoError(t, s.Unregister(ctx, "task-a"))

	val = testhelpers.GetGaugeValue(t, tc, "test_scheduler_tasks_registered")
	assert.Equal(t, float64(1), val, "should have 1 registered task after unregister")
}

func TestScheduler_Metrics_TaskExecution(t *testing.T) {
	tc := testhelpers.NewTestCollector()

	storage := mustNewMemory(t, 100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithCollector(tc),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var execCount atomic.Int32
	require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
		ID:         "metrics-task",
		Schedule:   "@every 1s",
		RunOnStart: true,
		Priority:   corescheduler.TaskPriorityNormal,
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	}))

	// Wait for at least one execution
	require.Eventually(t, func() bool { return execCount.Load() >= 1 },
		2*time.Second, 50*time.Millisecond)

	// Check tasksDispatched counter
	dispatched := testhelpers.GetCounterValue(t, tc, "test_scheduler_tasks_dispatched_total",
		"task_id", "metrics-task", "priority", "normal")
	assert.GreaterOrEqual(t, dispatched, float64(1), "tasks_dispatched_total should be >= 1")

	// Check taskDuration histogram has observations
	duration := testhelpers.GetHistogramCount(t, tc, "test_scheduler_task_duration_seconds",
		"task_id", "metrics-task", "priority", "normal")
	assert.GreaterOrEqual(t, duration, uint64(1), "task_duration_seconds should have >= 1 observation")

	// Check tickDuration has observations
	tickCount := testhelpers.GetHistogramCount(t, tc, "test_scheduler_tick_duration_seconds")
	assert.GreaterOrEqual(t, tickCount, uint64(1), "tick_duration_seconds should have >= 1 observation")
}

func TestScheduler_Metrics_TaskErrors(t *testing.T) {
	tc := testhelpers.NewTestCollector()

	storage := mustNewMemory(t, 100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithCollector(tc),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var execCount atomic.Int32
	require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
		ID:         "failing-task",
		Schedule:   "@every 1s",
		RunOnStart: true,
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return errors.New("task failed")
		},
	}))

	require.Eventually(t, func() bool { return execCount.Load() >= 1 },
		2*time.Second, 50*time.Millisecond)

	errorCount := testhelpers.GetCounterValue(t, tc, "test_scheduler_task_errors_total",
		"task_id", "failing-task")
	assert.GreaterOrEqual(t, errorCount, float64(1), "task_errors_total should be >= 1")
}
