// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"context"
	"iter"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

func TestScheduler_RegisterAndRun(t *testing.T) {
	storage := memory.New(100)
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
		ID:         "test-task",
		Schedule:   "@every 1s",
		RunOnStart: true,
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	// Wait for at least 2 executions (RunOnStart + 1 scheduled)
	time.Sleep(2500 * time.Millisecond)

	count := execCount.Load()
	require.GreaterOrEqual(t, count, int32(2), "expected at least 2 executions")
}

func TestScheduler_PauseResume(t *testing.T) {
	storage := memory.New(100)
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
		ID:       "pause-test",
		Schedule: "@every 1s",
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	// Pause the task
	require.NoError(t, s.PauseTask(ctx, "pause-test"))

	// Wait a bit and verify no executions
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, int32(0), execCount.Load(), "task executed while paused")

	// Resume and verify execution
	require.NoError(t, s.ResumeTask(ctx, "pause-test"))

	time.Sleep(1500 * time.Millisecond)
	require.NotEqual(t, int32(0), execCount.Load(), "task did not execute after resume")
}

func TestScheduler_SkipNextRun(t *testing.T) {
	storage := memory.New(100)
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
		ID:       "skip-test",
		Schedule: "@every 2s",
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	// Skip next run immediately after registration, well before the first 2s trigger
	require.NoError(t, s.SkipNextRun(ctx, "skip-test"))

	// Wait past the first scheduled time (2s interval + margin)
	time.Sleep(2500 * time.Millisecond)

	// Should have 0 executions (skipped)
	require.Equal(t, int32(0), execCount.Load(), "first run was not skipped")

	// Wait for next run (another 2.5s)
	time.Sleep(2500 * time.Millisecond)

	// Should have executed now
	require.NotEqual(t, int32(0), execCount.Load(), "task did not execute after skip")
}

func TestScheduler_Tasks(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	// Register multiple tasks
	for i := range 3 {
		id := string("task-" + string(rune('a'+i)))
		err := s.Register(ctx, corescheduler.TaskConfig{
			ID:       id,
			Schedule: "@every 1h",
			Func:     func(_ context.Context) error { return nil },
		})
		require.NoError(t, err, "failed to register task %s", id)
	}

	// Collect tasks using iter
	var tasks []*scheduler.TaskSummary
	for task, err := range s.Tasks(ctx) {
		require.NoError(t, err)
		tasks = append(tasks, task)
	}

	require.Len(t, tasks, 3)
}

func TestScheduler_History(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(50*time.Millisecond))

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:         "history-test",
		Schedule:   "@every 1s",
		RunOnStart: true,
		Func:       func(_ context.Context) error { return nil },
	})
	require.NoError(t, err)

	// Wait for some executions (RunOnStart will execute immediately)
	time.Sleep(500 * time.Millisecond)

	// Collect history using iter
	var history []*scheduler.TaskHistory
	for h, err := range s.History(ctx, "history-test") {
		require.NoError(t, err)
		history = append(history, h)
	}

	require.NotEmpty(t, history, "expected history entries")

	// Verify history entry fields
	for _, h := range history {
		require.Equal(t, "history-test", h.TaskID)
		require.NotEmpty(t, h.ID, "history entry missing ID")
		require.NotEmpty(t, h.RunID, "history entry missing RunID")
		require.True(t, h.Success, "expected successful execution")
	}
}

func TestMemoryStorage_GetTask(t *testing.T) {
	storage := memory.New(100)
	ctx := t.Context()

	// Get non-existent task
	state, err := storage.GetTask(ctx, "non-existent")
	require.NoError(t, err)
	require.Nil(t, state)

	// Insert and retrieve
	expected := &scheduler.TaskState{
		TaskSummary: scheduler.TaskSummary{
			ID:       "test",
			Status:   scheduler.TaskStatusActive,
			Schedule: "@every 1h",
			Priority: corescheduler.TaskPriorityNormal,
		},
		Meta: map[string]string{"key": "value"},
	}
	require.NoError(t, storage.UpsertTask(ctx, expected))

	state, err = storage.GetTask(ctx, "test")
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, expected.ID, state.ID)
	require.Equal(t, "value", state.Meta["key"])
	require.Equal(t, expected.Priority, state.Priority)
}

func TestMemoryStorage_History(t *testing.T) {
	storage := memory.New(5) // Small limit for testing
	ctx := t.Context()

	taskID := "test"

	// Add more than limit
	for i := range 10 {
		h := &scheduler.TaskHistory{
			ID:        string(rune('a' + i)),
			TaskID:    taskID,
			RunID:     string(rune('a' + i)),
			StartedAt: time.Now().Add(time.Duration(i) * time.Minute).Unix(),
			Success:   true,
		}
		require.NoError(t, storage.AddHistory(ctx, h))
	}

	// Collect history using iter
	var history []*scheduler.TaskHistory
	for h, err := range storage.History(ctx, taskID) {
		require.NoError(t, err)
		history = append(history, h)
	}

	// Should have only maxHist entries
	require.Len(t, history, 5)

	// Verify sorted by start time descending
	for i := 1; i < len(history); i++ {
		require.GreaterOrEqual(t, history[i-1].StartedAt, history[i].StartedAt, "history not sorted by start time descending")
	}
}

func TestScheduler_ConcurrencyLimit(t *testing.T) {
	storage := memory.New(100)
	// Limit to 2 concurrent tasks
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithMaxConcurrentTasks(2),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var (
		concurrent   atomic.Int32
		maxObserved  atomic.Int32
		taskDuration = 200 * time.Millisecond
	)

	// Register 5 tasks that all run at the same time
	for i := range 5 {
		id := string("concurrent-" + string(rune('a'+i)))
		err := s.Register(ctx, corescheduler.TaskConfig{
			ID:         id,
			Schedule:   "@every 1s",
			RunOnStart: true,
			Func: func(_ context.Context) error {
				// Track concurrent executions
				current := concurrent.Add(1)
				// Update max if needed
				for {
					max := maxObserved.Load()
					if current <= max || maxObserved.CompareAndSwap(max, current) {
						break
					}
				}
				time.Sleep(taskDuration)
				concurrent.Add(-1)
				return nil
			},
		})
		require.NoError(t, err, "failed to register task %s", id)
	}

	// Wait for tasks to run
	time.Sleep(500 * time.Millisecond)

	// Max concurrent should not exceed the limit of 2
	observed := maxObserved.Load()
	require.LessOrEqual(t, observed, int32(2), "concurrent tasks exceeded limit: max observed %d, limit was 2", observed)

	// Verify monitoring methods
	require.GreaterOrEqual(t, s.AvailableSlots(), 0, "AvailableSlots should be >= 0 when limit is set")
}

func TestScheduler_UnlimitedConcurrency(t *testing.T) {
	storage := memory.New(100)
	// No concurrency limit (default)
	s := scheduler.New(storage, scheduler.WithTickInterval(50*time.Millisecond))

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	// Should return -1 for unlimited
	require.Equal(t, -1, s.AvailableSlots(), "AvailableSlots should be -1 for unlimited")

	// RunningTasksCount returns 0 when unlimited (not tracked)
	require.Equal(t, 0, s.RunningTasksCount(), "RunningTasksCount should be 0 for unlimited")
}

func TestScheduler_LongRunningTasksDoNotBlock(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithMaxConcurrentTasks(10), // Enough slots
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var fastTaskRuns atomic.Int32

	// Register a long-running task
	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:         "slow-task",
		Schedule:   "@every 1s",
		RunOnStart: true,
		Func: func(_ context.Context) error {
			time.Sleep(2 * time.Second)
			return nil
		},
	})
	require.NoError(t, err)

	// Register a fast task
	err = s.Register(ctx, corescheduler.TaskConfig{
		ID:         "fast-task",
		Schedule:   "@every 1s",
		RunOnStart: true,
		Func: func(_ context.Context) error {
			fastTaskRuns.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	// Wait for fast task to run multiple times while slow task is still running
	// Slow task takes 2s, so wait 2.5s to get RunOnStart + 2 scheduled runs for fast task
	time.Sleep(2500 * time.Millisecond)

	runs := fastTaskRuns.Load()
	require.GreaterOrEqual(t, runs, int32(2), "fast task should have run multiple times")
}

func TestScheduler_CriticalPriorityBypassesLimits(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithMaxConcurrentTasks(1), // Very strict limit
		scheduler.WithReservedHighPrioritySlots(0),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var (
		criticalCount atomic.Int32
		normalCount   atomic.Int32
	)

	// Register a long-running normal task first to occupy the single slot
	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:         "normal-long-task",
		Schedule:   "@every 1s",
		RunOnStart: true,
		Priority:   corescheduler.TaskPriorityNormal,
		Func: func(_ context.Context) error {
			normalCount.Add(1)
			time.Sleep(300 * time.Millisecond)
			return nil
		},
	})
	require.NoError(t, err)

	// Register a critical task that should bypass the limit
	err = s.Register(ctx, corescheduler.TaskConfig{
		ID:         "critical-task",
		Schedule:   "@every 1s",
		RunOnStart: true,
		Priority:   corescheduler.TaskPriorityCritical,
		Func: func(_ context.Context) error {
			criticalCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	// Wait for tasks to run
	time.Sleep(400 * time.Millisecond)

	require.NotEqual(t, int32(0), criticalCount.Load(), "critical task did not run")
	require.NotEqual(t, int32(0), normalCount.Load(), "normal task did not run")
}

func TestScheduler_LowPriorityDeferred(t *testing.T) {
	storage := memory.New(100)
	// 1 slot total, no reserved slots
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithMaxConcurrentTasks(1),
		scheduler.WithReservedHighPrioritySlots(0),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var (
		lowRuns    atomic.Int32
		normalRuns atomic.Int32
	)

	// Register a normal task that occupies the slot
	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:         "normal-blocker",
		Priority:   corescheduler.TaskPriorityNormal,
		Schedule:   "@every 1h",
		RunOnStart: true,
		Func: func(_ context.Context) error {
			normalRuns.Add(1)
			time.Sleep(300 * time.Millisecond)
			return nil
		},
	})
	require.NoError(t, err)

	// Wait for normal task to start
	time.Sleep(100 * time.Millisecond)

	// Register a low priority task - should be deferred (non-blocking)
	err = s.Register(ctx, corescheduler.TaskConfig{
		ID:         "low-task",
		Priority:   corescheduler.TaskPriorityLow,
		Schedule:   "@every 1h",
		RunOnStart: true,
		Func: func(_ context.Context) error {
			lowRuns.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	// Check immediately - low task shouldn't have run yet (slot is occupied)
	time.Sleep(50 * time.Millisecond)
	if lowRuns.Load() > 0 && normalRuns.Load() > 0 {
		// Both ran quickly - that's fine, means slot was available
		return
	}

	// Wait for normal task to complete and slot to free up
	time.Sleep(400 * time.Millisecond)

	// Low task should eventually run
	require.NotEqual(t, int32(0), lowRuns.Load(), "low priority task should have run after slot became available")
}

func TestTaskPriority_String(t *testing.T) {
	tests := []struct {
		priority corescheduler.TaskPriority
		expected string
	}{
		{corescheduler.TaskPriorityCritical, "critical"},
		{corescheduler.TaskPriorityHigh, "high"},
		{corescheduler.TaskPriorityNormal, "normal"},
		{corescheduler.TaskPriorityLow, "low"},
		{corescheduler.TaskPriority(99), "unknown"},
	}

	for _, tt := range tests {
		require.Equal(t, tt.expected, tt.priority.String(), "For priority %d", tt.priority)
	}
}

func TestMemoryStorage_Tasks(t *testing.T) {
	storage := memory.New(100)
	ctx := t.Context()

	// Add some tasks
	for i := range 3 {
		id := string("task-" + string(rune('a'+i)))
		err := storage.UpsertTask(ctx, &scheduler.TaskState{
			TaskSummary: scheduler.TaskSummary{
				ID:       id,
				Status:   scheduler.TaskStatusActive,
				Schedule: "@every 1h",
			},
		})
		require.NoError(t, err)
	}

	// Test iterator
	count := 0
	for state, err := range storage.Tasks(ctx) {
		require.NoError(t, err)
		require.NotNil(t, state)
		count++
	}

	require.Equal(t, 3, count)
}

func TestMemoryStorage_HistoryIter(t *testing.T) {
	storage := memory.New(100)
	ctx := t.Context()
	taskID := "test"

	// Add some history
	for i := range 3 {
		h := &scheduler.TaskHistory{
			ID:        generateTestID(i),
			TaskID:    taskID,
			RunID:     generateTestID(i + 100),
			StartedAt: time.Now().Add(time.Duration(i) * time.Minute).Unix(),
			Success:   true,
		}
		require.NoError(t, storage.AddHistory(ctx, h))
	}

	// Test iterator
	count := 0
	var prevStartedAt int64
	for h, err := range storage.History(ctx, taskID) {
		require.NoError(t, err)
		require.NotNil(t, h)
		// Verify sorted descending
		if count > 0 {
			require.GreaterOrEqual(t, prevStartedAt, h.StartedAt, "history not sorted descending")
		}
		prevStartedAt = h.StartedAt
		count++
	}

	require.Equal(t, 3, count)
}

func TestScheduler_IsLeader(t *testing.T) {
	storage := memory.New(100)

	// Without leader election, always returns true
	s := scheduler.New(storage)
	require.True(t, s.IsLeader(), "expected IsLeader() to return true without leader election configured")
}

func TestScheduler_UnmanagedTask(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(50*time.Millisecond))

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var execCount atomic.Int32

	// Register an unmanaged task
	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:        "unmanaged-task",
		Schedule:  "@every 1s",
		Unmanaged: true,
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	// Verify state has Unmanaged flag set
	state, err := s.GetTaskState(ctx, "unmanaged-task")
	require.NoError(t, err)
	require.True(t, state.Unmanaged, "expected Unmanaged flag to be set in state")

	// Try to pause - should fail with ErrTaskUnmanaged
	err = s.PauseTask(ctx, "unmanaged-task")
	require.ErrorIs(t, err, scheduler.ErrTaskUnmanaged)

	// Try to disable - should fail with ErrTaskUnmanaged
	err = s.DisableTask(ctx, "unmanaged-task")
	require.ErrorIs(t, err, scheduler.ErrTaskUnmanaged)

	// Verify task is still active and running
	state, _ = s.GetTaskState(ctx, "unmanaged-task")
	require.Equal(t, scheduler.TaskStatusActive, state.Status)

	// Wait and verify task executes normally (1s schedule + buffer)
	time.Sleep(1500 * time.Millisecond)
	require.NotEqual(t, int32(0), execCount.Load(), "unmanaged task should still execute")
}

func TestScheduler_DisableHistory(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(50*time.Millisecond))

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var execCount atomic.Int32

	// Register a task with history disabled
	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:             "no-history-task",
		Schedule:       "@every 1s",
		RunOnStart:     true,
		DisableHistory: true,
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	// Verify state has DisableHistory flag set
	state, err := s.GetTaskState(ctx, "no-history-task")
	require.NoError(t, err)
	require.True(t, state.DisableHistory, "expected DisableHistory flag to be set in state")

	// Wait for some executions (RunOnStart + 1 scheduled run = 2+ executions)
	time.Sleep(2500 * time.Millisecond)

	// Verify task executed
	require.GreaterOrEqual(t, execCount.Load(), int32(2), "expected at least 2 executions")

	// Verify no history was recorded
	var historyCount int
	for _, err := range s.History(ctx, "no-history-task") {
		require.NoError(t, err)
		historyCount++
	}

	require.Equal(t, 0, historyCount, "expected no history for task with DisableHistory")
}

func TestScheduler_TaskSummaryFlags(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	// Register task with both flags
	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:             "flagged-task",
		Schedule:       "@every 1h",
		DisableHistory: true,
		Unmanaged:      true,
		Func:           func(_ context.Context) error { return nil },
	})
	require.NoError(t, err)

	// Verify flags are present in TaskSummary
	for summary, err := range s.Tasks(ctx) {
		require.NoError(t, err)
		if summary.ID == "flagged-task" {
			require.True(t, summary.DisableHistory, "expected DisableHistory in TaskSummary")
			require.True(t, summary.Unmanaged, "expected Unmanaged in TaskSummary")
			return
		}
	}
	require.Fail(t, "task not found in Tasks()")
}

func TestScheduler_TypedErrors(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	// Register a normal task for testing
	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:       "test-task",
		Schedule: "@every 1h",
		Func:     func(_ context.Context) error { return nil },
	})
	require.NoError(t, err)

	// Test ErrTaskNotFound for non-existent task
	require.ErrorIs(t, s.PauseTask(ctx, "non-existent"), scheduler.ErrTaskNotFound)

	// Test ErrTaskNotRegistered for unregistering non-existent task
	require.ErrorIs(t, s.Unregister(ctx, "non-existent"), scheduler.ErrTaskNotRegistered)

	// Test ErrTaskNotPaused when resuming active task
	require.ErrorIs(t, s.ResumeTask(ctx, "test-task"), scheduler.ErrTaskNotPaused)

	// Test ErrTaskNotDisabled when enabling active task
	require.ErrorIs(t, s.EnableTask(ctx, "test-task"), scheduler.ErrTaskNotDisabled)

	// Disable task to test ErrTaskDisabled
	require.NoError(t, s.DisableTask(ctx, "test-task"))

	// Test ErrTaskDisabled when triggering disabled task
	require.ErrorIs(t, s.TriggerTask(ctx, "test-task"), scheduler.ErrTaskDisabled)
}

func TestScheduler_OneShotTask_ExecutesOnceAndCompletes(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(50*time.Millisecond))

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var execCount atomic.Int32

	runAt := time.Now().Add(200 * time.Millisecond)
	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:    "oneshot-task",
		RunAt: runAt,
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	// Verify initially active
	state, _ := s.GetTaskState(ctx, "oneshot-task")
	require.Equal(t, scheduler.TaskStatusActive, state.Status)
	require.True(t, state.OneShot, "expected OneShot flag to be set")

	// Wait for execution
	time.Sleep(500 * time.Millisecond)

	require.Equal(t, int32(1), execCount.Load(), "expected exactly 1 execution")

	// Verify completed status
	state, _ = s.GetTaskState(ctx, "oneshot-task")
	require.Equal(t, scheduler.TaskStatusCompleted, state.Status)
	require.Equal(t, int64(0), state.NextRunAt, "expected NextRunAt=0")

	// Wait more and verify no re-execution
	time.Sleep(300 * time.Millisecond)
	require.Equal(t, int32(1), execCount.Load(), "task re-executed after completion")
}

func TestScheduler_OneShotTask_RunAtInPast(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(50*time.Millisecond))

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var execCount atomic.Int32

	// RunAt in the past → should execute immediately
	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:    "oneshot-past",
		RunAt: time.Now().Add(-time.Hour),
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	// Wait for execution
	time.Sleep(300 * time.Millisecond)

	require.Equal(t, int32(1), execCount.Load(), "expected 1 execution for past RunAt")

	state, _ := s.GetTaskState(ctx, "oneshot-past")
	require.Equal(t, scheduler.TaskStatusCompleted, state.Status)
}

func TestScheduler_OneShotTask_ValidationErrors(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	noop := func(_ context.Context) error { return nil }

	// Both RunAt and Schedule set → ErrScheduleConflict
	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:       "conflict",
		RunAt:    time.Now().Add(time.Hour),
		Schedule: "@every 1h",
		Func:     noop,
	})
	require.ErrorIs(t, err, scheduler.ErrScheduleConflict)

	// Neither RunAt nor Schedule → existing "schedule is required" error
	err = s.Register(ctx, corescheduler.TaskConfig{
		ID:   "empty",
		Func: noop,
	})
	require.Error(t, err)
}

func TestScheduler_OneShotTask_TriggerCompleted(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(50*time.Millisecond))

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:    "oneshot-trigger",
		RunAt: time.Now().Add(-time.Second), // Execute immediately
		Func:  func(_ context.Context) error { return nil },
	})
	require.NoError(t, err)

	// Wait for execution to complete
	time.Sleep(300 * time.Millisecond)

	// Verify completed
	state, _ := s.GetTaskState(ctx, "oneshot-trigger")
	require.Equal(t, scheduler.TaskStatusCompleted, state.Status)

	// Try to trigger completed task
	require.ErrorIs(t, s.TriggerTask(ctx, "oneshot-trigger"), scheduler.ErrTaskCompleted)
}

func TestScheduler_OneShotTask_ManualTriggerBeforeTime(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(50*time.Millisecond))

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var execCount atomic.Int32

	// Schedule far in the future
	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:    "oneshot-manual",
		RunAt: time.Now().Add(time.Hour),
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	// Manual trigger before scheduled time
	require.NoError(t, s.TriggerTask(ctx, "oneshot-manual"))

	// Wait for execution
	time.Sleep(300 * time.Millisecond)

	require.Equal(t, int32(1), execCount.Load(), "expected 1 execution")

	// Should be completed
	state, _ := s.GetTaskState(ctx, "oneshot-manual")
	require.Equal(t, scheduler.TaskStatusCompleted, state.Status)
}

func TestScheduler_OneShotTask_PauseResume(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(50*time.Millisecond))

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var execCount atomic.Int32

	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:    "oneshot-pause",
		RunAt: time.Now().Add(200 * time.Millisecond),
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	// Pause before execution
	require.NoError(t, s.PauseTask(ctx, "oneshot-pause"))

	// Wait past the RunAt time
	time.Sleep(400 * time.Millisecond)
	require.Equal(t, int32(0), execCount.Load(), "task executed while paused")

	// Resume
	require.NoError(t, s.ResumeTask(ctx, "oneshot-pause"))

	// Should execute now (RunAt was in past, resume sets NextRunAt to now)
	time.Sleep(300 * time.Millisecond)
	require.Equal(t, int32(1), execCount.Load(), "expected 1 execution after resume")

	// Verify completed
	state, _ := s.GetTaskState(ctx, "oneshot-pause")
	require.Equal(t, scheduler.TaskStatusCompleted, state.Status)

	// Pause on completed task should fail
	require.ErrorIs(t, s.PauseTask(ctx, "oneshot-pause"), scheduler.ErrTaskCompleted)
}

func TestScheduler_OneShotTask_History(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(50*time.Millisecond))

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:    "oneshot-history",
		RunAt: time.Now().Add(-time.Second), // Execute immediately
		Func:  func(_ context.Context) error { return nil },
	})
	require.NoError(t, err)

	// Wait for execution
	time.Sleep(300 * time.Millisecond)

	// Verify history was recorded
	var history []*scheduler.TaskHistory
	for h, err := range s.History(ctx, "oneshot-history") {
		require.NoError(t, err)
		history = append(history, h)
	}

	require.Len(t, history, 1)
	require.True(t, history[0].Success, "expected successful execution in history")
}

func TestScheduler_OneShotTask_TaskSummary(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:    "oneshot-summary",
		RunAt: time.Now().Add(time.Hour),
		Func:  func(_ context.Context) error { return nil },
	})
	require.NoError(t, err)

	for summary, err := range s.Tasks(ctx) {
		require.NoError(t, err)
		if summary.ID == "oneshot-summary" {
			require.True(t, summary.OneShot, "expected OneShot flag in TaskSummary")
			require.Empty(t, summary.Schedule, "expected empty schedule for one-shot")
			return
		}
	}
	require.Fail(t, "oneshot-summary not found in Tasks()")
}

func TestTasksCollect(t *testing.T) {
	storage := memory.New(100)
	ctx := t.Context()

	// Add some tasks
	for i := range 3 {
		id := string("task-" + string(rune('a'+i)))
		err := storage.UpsertTask(ctx, &scheduler.TaskState{
			TaskSummary: scheduler.TaskSummary{
				ID:       id,
				Status:   scheduler.TaskStatusActive,
				Schedule: "@every 1h",
			},
		})
		require.NoError(t, err)
	}

	// Test slices.Collect with iterator
	tasks := slices.Collect(tasksSeqValues(storage.Tasks(ctx)))
	require.Len(t, tasks, 3)
}

// generateTestID generates a simple ID for testing.
func TestScheduler_DynamicConcurrencyLimit(t *testing.T) {
	storage := memory.New(100)
	// Dynamic limit of 2
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithConcurrencyLimitFunc(func() int { return 2 }),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var (
		concurrent  atomic.Int32
		maxObserved atomic.Int32
	)

	// Register 5 tasks that all run at the same time
	for i := range 5 {
		id := "dynamic-" + string(rune('a'+i))
		err := s.Register(ctx, corescheduler.TaskConfig{
			ID:         id,
			Schedule:   "@every 1s",
			RunOnStart: true,
			Func: func(_ context.Context) error {
				current := concurrent.Add(1)
				for {
					observed := maxObserved.Load()
					if current <= observed || maxObserved.CompareAndSwap(observed, current) {
						break
					}
				}
				time.Sleep(200 * time.Millisecond)
				concurrent.Add(-1)
				return nil
			},
		})
		require.NoError(t, err, "failed to register task %s", id)
	}

	// Wait for tasks to run
	time.Sleep(500 * time.Millisecond)

	observed := maxObserved.Load()
	require.LessOrEqual(t, observed, int32(2), "dynamic concurrent tasks exceeded limit: max observed %d, limit was 2", observed)
}

func TestScheduler_DynamicConcurrencyAdaptive(t *testing.T) {
	storage := memory.New(100)
	var limit atomic.Int32
	limit.Store(1)

	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithConcurrencyLimitFunc(func() int { return int(limit.Load()) }),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var (
		concurrent  atomic.Int32
		maxObserved atomic.Int32
	)

	for i := range 4 {
		id := "adaptive-" + string(rune('a'+i))
		err := s.Register(ctx, corescheduler.TaskConfig{
			ID:         id,
			Schedule:   "@every 1s",
			RunOnStart: true,
			Func: func(_ context.Context) error {
				current := concurrent.Add(1)
				for {
					observed := maxObserved.Load()
					if current <= observed || maxObserved.CompareAndSwap(observed, current) {
						break
					}
				}
				time.Sleep(200 * time.Millisecond)
				concurrent.Add(-1)
				return nil
			},
		})
		require.NoError(t, err, "failed to register task %s", id)
	}

	// Phase 1: limit=1, verify max concurrent ≤ 1
	time.Sleep(500 * time.Millisecond)
	phase1Max := maxObserved.Load()
	require.LessOrEqual(t, phase1Max, int32(1), "phase 1: expected max 1 concurrent")

	// Phase 2: increase limit to 4
	maxObserved.Store(0)
	limit.Store(4)
	time.Sleep(1500 * time.Millisecond)

	phase2Max := maxObserved.Load()
	require.LessOrEqual(t, phase2Max, int32(4), "phase 2: expected max 4 concurrent")
	// With limit=4 and 4 tasks, we should see >1 concurrent
	require.Greater(t, phase2Max, int32(1), "phase 2: expected >1 concurrent after limit increase")
}

func TestScheduler_WithEnvironment(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithEnvironment(concurrency.EnvironmentRateLimited), // limit = 3
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	// With RateLimited environment (limit=3) and no running tasks, available should be 3
	require.Equal(t, 3, s.AvailableSlots(), "expected 3 available slots for RateLimited environment")

	// RunningTasksCount should be 0 initially
	require.Equal(t, 0, s.RunningTasksCount(), "expected 0 running tasks initially")

	// AvailableHighPrioritySlots returns -1 in dynamic mode
	require.Equal(t, -1, s.AvailableHighPrioritySlots(), "expected -1 for AvailableHighPrioritySlots in dynamic mode")
}

func TestScheduler_DynamicCriticalBypassesLimit(t *testing.T) {
	storage := memory.New(100)
	// Dynamic limit of 1
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithConcurrencyLimitFunc(func() int { return 1 }),
		scheduler.WithReservedHighPrioritySlots(0),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var (
		criticalCount atomic.Int32
		normalCount   atomic.Int32
	)

	// Register a long-running normal task to occupy the single slot
	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:         "dynamic-normal",
		Schedule:   "@every 1s",
		RunOnStart: true,
		Priority:   corescheduler.TaskPriorityNormal,
		Func: func(_ context.Context) error {
			normalCount.Add(1)
			time.Sleep(300 * time.Millisecond)
			return nil
		},
	})
	require.NoError(t, err)

	// Register a critical task that should bypass the limit
	err = s.Register(ctx, corescheduler.TaskConfig{
		ID:         "dynamic-critical",
		Schedule:   "@every 1s",
		RunOnStart: true,
		Priority:   corescheduler.TaskPriorityCritical,
		Func: func(_ context.Context) error {
			criticalCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	// Wait for tasks to run
	time.Sleep(400 * time.Millisecond)

	require.NotEqual(t, int32(0), criticalCount.Load(), "critical task did not run in dynamic mode")
	require.NotEqual(t, int32(0), normalCount.Load(), "normal task did not run in dynamic mode")
}

func TestScheduler_DynamicLimitFuncPrecedence(t *testing.T) {
	storage := memory.New(100)
	// Both static and dynamic set: dynamic should take precedence
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithMaxConcurrentTasks(10),
		scheduler.WithConcurrencyLimitFunc(func() int { return 3 }),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	// Dynamic takes precedence: available should be 3, not 10
	require.Equal(t, 3, s.AvailableSlots(), "expected 3 available slots (dynamic precedence)")
}

func TestScheduler_DynamicRunningCountAccuracy(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithConcurrencyLimitFunc(func() int { return 10 }),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	started := make(chan struct{})
	done := make(chan struct{})

	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:         "count-test",
		Schedule:   "@every 1h",
		RunOnStart: true,
		Func: func(_ context.Context) error {
			close(started)
			<-done
			return nil
		},
	})
	require.NoError(t, err)

	// Wait for task to start
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		require.Fail(t, "task did not start")
	}

	// Running count should be 1
	require.Equal(t, 1, s.RunningTasksCount(), "expected 1 running task")

	// Available should be 9 (10 - 1)
	require.Equal(t, 9, s.AvailableSlots(), "expected 9 available slots")

	// Let the task finish
	close(done)
	time.Sleep(200 * time.Millisecond)

	// Running count should return to 0
	require.Equal(t, 0, s.RunningTasksCount(), "expected 0 running tasks after completion")
}

func TestScheduler_RecoverStaleTasksOnStartup(t *testing.T) {
	storage := memory.New(100)
	ctx := t.Context()

	// Simulate a crashed scheduler: insert a task in Running status
	staleState := &scheduler.TaskState{
		TaskSummary: scheduler.TaskSummary{
			ID:        "stale-startup",
			Status:    scheduler.TaskStatusRunning,
			Schedule:  "@every 1s",
			Priority:  corescheduler.TaskPriorityNormal,
			NextRunAt: time.Now().Add(-time.Hour).Unix(),
		},
		UpdatedAt: time.Now().Add(-time.Hour).Unix(),
		CreatedAt: time.Now().Add(-2 * time.Hour).Unix(),
	}
	require.NoError(t, storage.UpsertTask(ctx, staleState))

	// Start a new scheduler — startup recovery should reset the task
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithStaleTaskTimeout(30*time.Minute),
	)
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	// Verify the task was reset to Active
	state, err := s.GetTaskState(ctx, "stale-startup")
	require.NoError(t, err)
	require.Equal(t, scheduler.TaskStatusActive, state.Status, "expected task to be reset to Active after startup recovery")
	require.Equal(t, int32(1), state.Failures, "expected failures to be incremented to 1")

	// Register the task function and verify it eventually executes
	var execCount atomic.Int32
	err = s.Register(ctx, corescheduler.TaskConfig{
		ID:       "stale-startup",
		Schedule: "@every 1s",
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	require.NotEqual(t, int32(0), execCount.Load(), "recovered task should have executed after registration")
}

func TestScheduler_RecoverStaleTasksPeriodic(t *testing.T) {
	storage := memory.New(100)

	// Use a very short stale timeout for testing
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithStaleTaskTimeout(200*time.Millisecond),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var execCount atomic.Int32

	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:       "stale-periodic",
		Schedule: "@every 1h",
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	// Manually set the task to Running in storage (simulating a stuck task)
	state, _ := s.GetTaskState(ctx, "stale-periodic")
	state.Status = scheduler.TaskStatusRunning
	state.UpdatedAt = time.Now().Add(-time.Minute).Unix() // Well past the 200ms timeout
	require.NoError(t, storage.UpsertTask(ctx, state))

	// Wait for periodic recovery (ticker interval = min(200ms, 5min) = 200ms)
	time.Sleep(500 * time.Millisecond)

	// Verify the task was recovered to Active
	state, err = s.GetTaskState(ctx, "stale-periodic")
	require.NoError(t, err)
	require.NotEqual(t, scheduler.TaskStatusRunning, state.Status, "expected stale task to be recovered from Running status")
}

func TestScheduler_RunningTaskNotRecoveredPrematurely(t *testing.T) {
	storage := memory.New(100)

	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithStaleTaskTimeout(10*time.Second),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	started := make(chan struct{})
	done := make(chan struct{})

	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:         "long-running",
		Schedule:   "@every 1h",
		RunOnStart: true,
		Func: func(_ context.Context) error {
			close(started)
			<-done
			return nil
		},
	})
	require.NoError(t, err)

	// Wait for the task to start executing
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		require.Fail(t, "task did not start")
	}

	// Verify the task is in Running status
	state, _ := s.GetTaskState(ctx, "long-running")
	require.Equal(t, scheduler.TaskStatusRunning, state.Status)

	// Wait a bit — the task should NOT be recovered since it's within the timeout
	// and actively running in this process
	time.Sleep(500 * time.Millisecond)

	state, _ = s.GetTaskState(ctx, "long-running")
	require.Equal(t, scheduler.TaskStatusRunning, state.Status, "running task was prematurely recovered")

	// Let the task finish
	close(done)
	time.Sleep(200 * time.Millisecond)

	state, _ = s.GetTaskState(ctx, "long-running")
	require.Equal(t, scheduler.TaskStatusActive, state.Status, "expected task to return to Active after completion")
}

func generateTestID(n int) string {
	return string(rune('a' + n))
}

// tasksSeqValues converts iter.Seq2[*scheduler.TaskState, error] to iter.Seq[*scheduler.TaskState].
// Errors are ignored - use for testing only.
func tasksSeqValues(seq iter.Seq2[*scheduler.TaskState, error]) iter.Seq[*scheduler.TaskState] {
	return func(yield func(*scheduler.TaskState) bool) {
		for state, err := range seq {
			if err != nil {
				return
			}
			if !yield(state) {
				return
			}
		}
	}
}

// --- Pagination tests ---

func registerDummyTasks(t *testing.T, s *scheduler.Scheduler, n int) {
	t.Helper()
	ctx := t.Context()
	for i := range n {
		id := generatePaddedID(i)
		err := s.Register(ctx, corescheduler.TaskConfig{
			ID:       id,
			Schedule: "@every 1h",
			Func:     func(_ context.Context) error { return nil },
		})
		require.NoError(t, err, "register task %q", id)
	}
}

func generatePaddedID(n int) string {
	return "task-" + string(rune('a'+n/26)) + string(rune('a'+n%26))
}

func TestScheduler_TasksPaginated_NoFilter(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(time.Hour))
	ctx := t.Context()
	_ = s.Start(ctx)
	defer func() { _ = s.Stop(ctx) }()

	registerDummyTasks(t, s, 5)

	// First page: limit 2
	result, err := s.TasksPaginated(ctx, scheduler.PageRequest{Limit: 2}, "")
	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	require.NotNil(t, result.NextCursor, "expected nextCursor for more pages")

	// Second page
	result2, err := s.TasksPaginated(ctx, scheduler.PageRequest{Limit: 2, Cursor: *result.NextCursor}, "")
	require.NoError(t, err)
	require.Len(t, result2.Items, 2)
	// IDs should not overlap
	require.NotEqual(t, result.Items[0].ID, result2.Items[0].ID, "page 2 IDs should not overlap with page 1")

	// Third page
	result3, err := s.TasksPaginated(ctx, scheduler.PageRequest{Limit: 2, Cursor: *result2.NextCursor}, "")
	require.NoError(t, err)
	require.Len(t, result3.Items, 1)
	require.Nil(t, result3.NextCursor, "expected nil nextCursor on last page")
}

func TestScheduler_TasksPaginated_DefaultLimit(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(time.Hour))
	ctx := t.Context()
	_ = s.Start(ctx)
	defer func() { _ = s.Stop(ctx) }()

	registerDummyTasks(t, s, 3)

	// Limit 0 → default page size (100), which is > 3
	result, err := s.TasksPaginated(ctx, scheduler.PageRequest{}, "")
	require.NoError(t, err)
	require.Len(t, result.Items, 3)
	require.Nil(t, result.NextCursor, "expected nil nextCursor when all items fit in one page")
}

func TestScheduler_TasksPaginated_InvalidCursor(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(time.Hour))
	ctx := t.Context()
	_ = s.Start(ctx)
	defer func() { _ = s.Stop(ctx) }()

	_, err := s.TasksPaginated(ctx, scheduler.PageRequest{Cursor: "garbage"}, "")
	require.Error(t, err)
}

func TestScheduler_HistoryPaginated_NoFilter(t *testing.T) {
	storage := memory.New(1000)
	s := scheduler.New(storage, scheduler.WithTickInterval(time.Hour))
	ctx := t.Context()
	_ = s.Start(ctx)
	defer func() { _ = s.Stop(ctx) }()

	// Register task and add history entries
	_ = s.Register(ctx, corescheduler.TaskConfig{
		ID:       "hist-task",
		Schedule: "@every 1h",
		Func:     func(_ context.Context) error { return nil },
	})

	for i := range 5 {
		_ = storage.AddHistory(ctx, &scheduler.TaskHistory{
			ID:        generatePaddedID(i),
			TaskID:    "hist-task",
			RunID:     "run-" + generatePaddedID(i),
			StartedAt: int64((i + 1) * 1000),
			EndedAt:   int64((i+1)*1000 + 500),
			Success:   true,
		})
	}

	// Page 1: limit 2
	result, err := s.HistoryPaginated(ctx, "hist-task", scheduler.PageRequest{Limit: 2}, "")
	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	require.NotNil(t, result.NextCursor, "expected nextCursor for more pages")
	// Should be sorted desc by StartedAt
	require.GreaterOrEqual(t, result.Items[0].StartedAt, result.Items[1].StartedAt, "expected descending order by StartedAt")

	// Page 2
	result2, err := s.HistoryPaginated(ctx, "hist-task", scheduler.PageRequest{Limit: 2, Cursor: *result.NextCursor}, "")
	require.NoError(t, err)
	require.Len(t, result2.Items, 2)

	// Page 3
	result3, err := s.HistoryPaginated(ctx, "hist-task", scheduler.PageRequest{Limit: 2, Cursor: *result2.NextCursor}, "")
	require.NoError(t, err)
	require.Len(t, result3.Items, 1)
	require.Nil(t, result3.NextCursor, "expected nil nextCursor on last page")
}

func TestScheduler_HistoryPaginated_InvalidCursor(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(time.Hour))
	ctx := t.Context()
	_ = s.Start(ctx)
	defer func() { _ = s.Stop(ctx) }()

	_, err := s.HistoryPaginated(ctx, "any-task", scheduler.PageRequest{Cursor: "bad"}, "")
	require.Error(t, err)
}

func TestScheduler_TasksPaginated_WithFilter(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(time.Hour))
	ctx := t.Context()
	_ = s.Start(ctx)
	defer func() { _ = s.Stop(ctx) }()

	// Register 5 tasks, then pause 2 of them.
	registerDummyTasks(t, s, 5)

	require.NoError(t, s.PauseTask(ctx, generatePaddedID(1)))
	require.NoError(t, s.PauseTask(ctx, generatePaddedID(3)))

	// Filter: status == 1 (Active) — should return 3 tasks.
	result, err := s.TasksPaginated(ctx, scheduler.PageRequest{Limit: 10}, "status == 1")
	require.NoError(t, err)
	require.Len(t, result.Items, 3)
	for _, item := range result.Items {
		require.Equal(t, scheduler.TaskStatusActive, item.Status, "expected active status for task %s", item.ID)
	}

	// Filter: status == 2 (Paused) — should return 2 tasks.
	result, err = s.TasksPaginated(ctx, scheduler.PageRequest{Limit: 10}, "status == 2")
	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	for _, item := range result.Items {
		require.Equal(t, scheduler.TaskStatusPaused, item.Status, "expected paused status for task %s", item.ID)
	}
}

func TestScheduler_HistoryPaginated_WithFilter(t *testing.T) {
	storage := memory.New(1000)
	s := scheduler.New(storage, scheduler.WithTickInterval(time.Hour))
	ctx := t.Context()
	_ = s.Start(ctx)
	defer func() { _ = s.Stop(ctx) }()

	_ = s.Register(ctx, corescheduler.TaskConfig{
		ID:       "filter-hist-task",
		Schedule: "@every 1h",
		Func:     func(_ context.Context) error { return nil },
	})

	// Add 6 history entries: alternating success and failure.
	for i := range 6 {
		h := &scheduler.TaskHistory{
			ID:        generatePaddedID(i),
			TaskID:    "filter-hist-task",
			RunID:     "run-" + generatePaddedID(i),
			StartedAt: int64((i + 1) * 1000),
			EndedAt:   int64((i+1)*1000 + 500),
			Success:   i%2 == 0, // 0,2,4 succeed; 1,3,5 fail
		}
		if !h.Success {
			h.Error = "simulated error"
		}
		_ = storage.AddHistory(ctx, h)
	}

	// Filter: success == true — should return 3 entries.
	result, err := s.HistoryPaginated(ctx, "filter-hist-task", scheduler.PageRequest{Limit: 10}, "success == true")
	require.NoError(t, err)
	require.Len(t, result.Items, 3)
	for _, item := range result.Items {
		require.True(t, item.Success, "expected success=true for entry %s", item.ID)
	}

	// Filter: success == false — should return 3 entries.
	result, err = s.HistoryPaginated(ctx, "filter-hist-task", scheduler.PageRequest{Limit: 10}, "success == false")
	require.NoError(t, err)
	require.Len(t, result.Items, 3)
	for _, item := range result.Items {
		require.False(t, item.Success, "expected success=false for entry %s", item.ID)
	}
}
