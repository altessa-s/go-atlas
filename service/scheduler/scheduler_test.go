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

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

func TestScheduler_RegisterAndRun(t *testing.T) {
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
		ID:         "test-task",
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

	// Wait for at least 2 executions (RunOnStart + 1 scheduled)
	time.Sleep(2500 * time.Millisecond)

	count := execCount.Load()
	if count < 2 {
		t.Errorf("expected at least 2 executions, got %d", count)
	}
}

func TestScheduler_PauseResume(t *testing.T) {
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
		ID:       "pause-test",
		Schedule: "@every 1s",
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Pause the task
	if err := s.PauseTask(ctx, "pause-test"); err != nil {
		t.Fatalf("failed to pause task: %v", err)
	}

	// Wait a bit and verify no executions
	time.Sleep(200 * time.Millisecond)
	if count := execCount.Load(); count > 0 {
		t.Errorf("task executed while paused, count: %d", count)
	}

	// Resume and verify execution
	if err := s.ResumeTask(ctx, "pause-test"); err != nil {
		t.Fatalf("failed to resume task: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)
	if count := execCount.Load(); count == 0 {
		t.Error("task did not execute after resume")
	}
}

func TestScheduler_SkipNextRun(t *testing.T) {
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
		ID:       "skip-test",
		Schedule: "@every 2s",
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Skip next run immediately after registration, well before the first 2s trigger
	if err := s.SkipNextRun(ctx, "skip-test"); err != nil {
		t.Fatalf("failed to skip next run: %v", err)
	}

	// Wait past the first scheduled time (2s interval + margin)
	time.Sleep(2500 * time.Millisecond)

	// Should have 0 executions (skipped)
	if count := execCount.Load(); count > 0 {
		t.Errorf("first run was not skipped, count: %d", count)
	}

	// Wait for next run (another 2.5s)
	time.Sleep(2500 * time.Millisecond)

	// Should have executed now
	if count := execCount.Load(); count == 0 {
		t.Error("task did not execute after skip")
	}
}

func TestScheduler_Tasks(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage)

	ctx := t.Context()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
		if err != nil {
			t.Fatalf("failed to register task %s: %v", id, err)
		}
	}

	// Collect tasks using iter
	var tasks []*scheduler.TaskSummary
	for task, err := range s.Tasks(ctx) {
		if err != nil {
			t.Fatalf("failed to iterate tasks: %v", err)
		}
		tasks = append(tasks, task)
	}

	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}
}

func TestScheduler_History(t *testing.T) {
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

	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:         "history-test",
		Schedule:   "@every 1s",
		RunOnStart: true,
		Func:       func(_ context.Context) error { return nil },
	})
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Wait for some executions (RunOnStart will execute immediately)
	time.Sleep(500 * time.Millisecond)

	// Collect history using iter
	var history []*scheduler.TaskHistory
	for h, err := range s.History(ctx, "history-test") {
		if err != nil {
			t.Fatalf("failed to iterate history: %v", err)
		}
		history = append(history, h)
	}

	if len(history) == 0 {
		t.Error("expected history entries, got none")
	}

	// Verify history entry fields
	for _, h := range history {
		if h.TaskID != "history-test" {
			t.Errorf("unexpected task ID in history: %s", h.TaskID)
		}
		if h.ID == "" || h.RunID == "" {
			t.Error("history entry missing ID or RunID")
		}
		if !h.Success {
			t.Error("expected successful execution")
		}
	}
}

func TestMemoryStorage_GetTask(t *testing.T) {
	storage := memory.New(100)
	ctx := t.Context()

	// Get non-existent task
	state, err := storage.GetTask(ctx, "non-existent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != nil {
		t.Error("expected nil for non-existent task")
	}

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
	if err := storage.UpsertTask(ctx, expected); err != nil {
		t.Fatalf("failed to upsert: %v", err)
	}

	state, err = storage.GetTask(ctx, "test")
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}
	if state == nil {
		t.Fatal("expected state, got nil")
	}
	if state.ID != expected.ID {
		t.Errorf("ID mismatch: %s != %s", state.ID, expected.ID)
	}
	if state.Meta["key"] != "value" {
		t.Error("meta not preserved")
	}
	if state.Priority != expected.Priority {
		t.Errorf("priority mismatch: %s != %s", state.Priority, expected.Priority)
	}
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
		if err := storage.AddHistory(ctx, h); err != nil {
			t.Fatalf("failed to add history: %v", err)
		}
	}

	// Collect history using iter
	var history []*scheduler.TaskHistory
	for h, err := range storage.History(ctx, taskID) {
		if err != nil {
			t.Fatalf("failed to iterate history: %v", err)
		}
		history = append(history, h)
	}

	// Should have only maxHist entries
	if len(history) != 5 {
		t.Errorf("expected 5 entries, got %d", len(history))
	}

	// Verify sorted by start time descending
	for i := 1; i < len(history); i++ {
		if history[i].StartedAt > history[i-1].StartedAt {
			t.Error("history not sorted by start time descending")
		}
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
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
		if err != nil {
			t.Fatalf("failed to register task %s: %v", id, err)
		}
	}

	// Wait for tasks to run
	time.Sleep(500 * time.Millisecond)

	// Max concurrent should not exceed the limit of 2
	observed := maxObserved.Load()
	if observed > 2 {
		t.Errorf("concurrent tasks exceeded limit: max observed %d, limit was 2", observed)
	}

	// Verify monitoring methods
	if s.AvailableSlots() < 0 {
		t.Error("AvailableSlots should be >= 0 when limit is set")
	}
}

func TestScheduler_UnlimitedConcurrency(t *testing.T) {
	storage := memory.New(100)
	// No concurrency limit (default)
	s := scheduler.New(storage, scheduler.WithTickInterval(50*time.Millisecond))

	ctx := t.Context()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	// Should return -1 for unlimited
	if s.AvailableSlots() != -1 {
		t.Errorf("AvailableSlots should be -1 for unlimited, got %d", s.AvailableSlots())
	}

	// RunningTasksCount returns 0 when unlimited (not tracked)
	if s.RunningTasksCount() != 0 {
		t.Errorf("RunningTasksCount should be 0 for unlimited, got %d", s.RunningTasksCount())
	}
}

func TestScheduler_LongRunningTasksDoNotBlock(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithMaxConcurrentTasks(10), // Enough slots
	)

	ctx := t.Context()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
	if err != nil {
		t.Fatalf("failed to register slow task: %v", err)
	}

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
	if err != nil {
		t.Fatalf("failed to register fast task: %v", err)
	}

	// Wait for fast task to run multiple times while slow task is still running
	// Slow task takes 2s, so wait 2.5s to get RunOnStart + 2 scheduled runs for fast task
	time.Sleep(2500 * time.Millisecond)

	runs := fastTaskRuns.Load()
	if runs < 2 {
		t.Errorf("fast task should have run multiple times, but only ran %d times", runs)
	}
}

func TestScheduler_CriticalPriorityBypassesLimits(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithMaxConcurrentTasks(1), // Very strict limit
		scheduler.WithReservedHighPrioritySlots(0),
	)

	ctx := t.Context()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
	if err != nil {
		t.Fatalf("failed to register normal task: %v", err)
	}

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
	if err != nil {
		t.Fatalf("failed to register critical task: %v", err)
	}

	// Wait for tasks to run
	time.Sleep(400 * time.Millisecond)

	if criticalCount.Load() == 0 {
		t.Error("critical task did not run")
	}
	if normalCount.Load() == 0 {
		t.Error("normal task did not run")
	}
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
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
	if err != nil {
		t.Fatalf("failed to register normal task: %v", err)
	}

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
	if err != nil {
		t.Fatalf("failed to register low task: %v", err)
	}

	// Check immediately - low task shouldn't have run yet (slot is occupied)
	time.Sleep(50 * time.Millisecond)
	if lowRuns.Load() > 0 && normalRuns.Load() > 0 {
		// Both ran quickly - that's fine, means slot was available
		return
	}

	// Wait for normal task to complete and slot to free up
	time.Sleep(400 * time.Millisecond)

	// Low task should eventually run
	if lowRuns.Load() == 0 {
		t.Error("low priority task should have run after slot became available")
	}
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
		if tt.priority.String() != tt.expected {
			t.Errorf("For priority %d, expected %s but got %s", tt.priority, tt.expected, tt.priority.String())
		}
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
		if err != nil {
			t.Fatalf("failed to upsert task: %v", err)
		}
	}

	// Test iterator
	count := 0
	for state, err := range storage.Tasks(ctx) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if state == nil {
			t.Fatal("unexpected nil state")
		}
		count++
	}

	if count != 3 {
		t.Errorf("expected 3 tasks from iterator, got %d", count)
	}
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
		if err := storage.AddHistory(ctx, h); err != nil {
			t.Fatalf("failed to add history: %v", err)
		}
	}

	// Test iterator
	count := 0
	var prevStartedAt int64
	for h, err := range storage.History(ctx, taskID) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if h == nil {
			t.Fatal("unexpected nil history")
		}
		// Verify sorted descending
		if count > 0 && h.StartedAt > prevStartedAt {
			t.Error("history not sorted descending")
		}
		prevStartedAt = h.StartedAt
		count++
	}

	if count != 3 {
		t.Errorf("expected 3 history entries from iterator, got %d", count)
	}
}

func TestScheduler_IsLeader(t *testing.T) {
	storage := memory.New(100)

	// Without leader election, always returns true
	s := scheduler.New(storage)
	if !s.IsLeader() {
		t.Error("expected IsLeader() to return true without leader election configured")
	}
}

func TestScheduler_UnmanagedTask(t *testing.T) {
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
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Verify state has Unmanaged flag set
	state, err := s.GetTaskState(ctx, "unmanaged-task")
	if err != nil {
		t.Fatalf("failed to get task state: %v", err)
	}
	if !state.Unmanaged {
		t.Error("expected Unmanaged flag to be set in state")
	}

	// Try to pause - should fail with ErrTaskUnmanaged
	err = s.PauseTask(ctx, "unmanaged-task")
	if err == nil {
		t.Error("expected error when pausing unmanaged task")
	}
	if err != scheduler.ErrTaskUnmanaged {
		t.Errorf("expected ErrTaskUnmanaged, got %v", err)
	}

	// Try to disable - should fail with ErrTaskUnmanaged
	err = s.DisableTask(ctx, "unmanaged-task")
	if err == nil {
		t.Error("expected error when disabling unmanaged task")
	}
	if err != scheduler.ErrTaskUnmanaged {
		t.Errorf("expected ErrTaskUnmanaged, got %v", err)
	}

	// Verify task is still active and running
	state, _ = s.GetTaskState(ctx, "unmanaged-task")
	if state.Status != scheduler.TaskStatusActive {
		t.Errorf("expected task to still be active, got %s", state.Status)
	}

	// Wait and verify task executes normally (1s schedule + buffer)
	time.Sleep(1500 * time.Millisecond)
	if execCount.Load() == 0 {
		t.Error("unmanaged task should still execute")
	}
}

func TestScheduler_DisableHistory(t *testing.T) {
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
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Verify state has DisableHistory flag set
	state, err := s.GetTaskState(ctx, "no-history-task")
	if err != nil {
		t.Fatalf("failed to get task state: %v", err)
	}
	if !state.DisableHistory {
		t.Error("expected DisableHistory flag to be set in state")
	}

	// Wait for some executions (RunOnStart + 1 scheduled run = 2+ executions)
	time.Sleep(2500 * time.Millisecond)

	// Verify task executed
	if execCount.Load() < 2 {
		t.Errorf("expected at least 2 executions, got %d", execCount.Load())
	}

	// Verify no history was recorded
	var historyCount int
	for _, err := range s.History(ctx, "no-history-task") {
		if err != nil {
			t.Fatalf("failed to iterate history: %v", err)
		}
		historyCount++
	}

	if historyCount > 0 {
		t.Errorf("expected no history for task with DisableHistory, got %d entries", historyCount)
	}
}

func TestScheduler_TaskSummaryFlags(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage)

	ctx := t.Context()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Verify flags are present in TaskSummary
	for summary, err := range s.Tasks(ctx) {
		if err != nil {
			t.Fatalf("failed to iterate tasks: %v", err)
		}
		if summary.ID == "flagged-task" {
			if !summary.DisableHistory {
				t.Error("expected DisableHistory in TaskSummary")
			}
			if !summary.Unmanaged {
				t.Error("expected Unmanaged in TaskSummary")
			}
			return
		}
	}
	t.Error("task not found in Tasks()")
}

func TestScheduler_TypedErrors(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage)

	ctx := t.Context()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Test ErrTaskNotFound for non-existent task
	if err := s.PauseTask(ctx, "non-existent"); err != scheduler.ErrTaskNotFound {
		t.Errorf("expected ErrTaskNotFound, got %v", err)
	}

	// Test ErrTaskNotRegistered for unregistering non-existent task
	if err := s.Unregister(ctx, "non-existent"); err != scheduler.ErrTaskNotRegistered {
		t.Errorf("expected ErrTaskNotRegistered, got %v", err)
	}

	// Test ErrTaskNotPaused when resuming active task
	if err := s.ResumeTask(ctx, "test-task"); err != scheduler.ErrTaskNotPaused {
		t.Errorf("expected ErrTaskNotPaused, got %v", err)
	}

	// Test ErrTaskNotDisabled when enabling active task
	if err := s.EnableTask(ctx, "test-task"); err != scheduler.ErrTaskNotDisabled {
		t.Errorf("expected ErrTaskNotDisabled, got %v", err)
	}

	// Disable task to test ErrTaskDisabled
	if err := s.DisableTask(ctx, "test-task"); err != nil {
		t.Fatalf("failed to disable task: %v", err)
	}

	// Test ErrTaskDisabled when triggering disabled task
	if err := s.TriggerTask(ctx, "test-task"); err != scheduler.ErrTaskDisabled {
		t.Errorf("expected ErrTaskDisabled, got %v", err)
	}
}

func TestScheduler_OneShotTask_ExecutesOnceAndCompletes(t *testing.T) {
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

	runAt := time.Now().Add(200 * time.Millisecond)
	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:    "oneshot-task",
		RunAt: runAt,
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("failed to register one-shot task: %v", err)
	}

	// Verify initially active
	state, _ := s.GetTaskState(ctx, "oneshot-task")
	if state.Status != scheduler.TaskStatusActive {
		t.Errorf("expected active status, got %s", state.Status)
	}
	if !state.OneShot {
		t.Error("expected OneShot flag to be set")
	}

	// Wait for execution
	time.Sleep(500 * time.Millisecond)

	if execCount.Load() != 1 {
		t.Errorf("expected exactly 1 execution, got %d", execCount.Load())
	}

	// Verify completed status
	state, _ = s.GetTaskState(ctx, "oneshot-task")
	if state.Status != scheduler.TaskStatusCompleted {
		t.Errorf("expected completed status, got %s", state.Status)
	}
	if state.NextRunAt != 0 {
		t.Errorf("expected NextRunAt=0, got %d", state.NextRunAt)
	}

	// Wait more and verify no re-execution
	time.Sleep(300 * time.Millisecond)
	if execCount.Load() != 1 {
		t.Errorf("task re-executed after completion, count: %d", execCount.Load())
	}
}

func TestScheduler_OneShotTask_RunAtInPast(t *testing.T) {
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

	// RunAt in the past → should execute immediately
	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:    "oneshot-past",
		RunAt: time.Now().Add(-time.Hour),
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("failed to register one-shot task: %v", err)
	}

	// Wait for execution
	time.Sleep(300 * time.Millisecond)

	if execCount.Load() != 1 {
		t.Errorf("expected 1 execution for past RunAt, got %d", execCount.Load())
	}

	state, _ := s.GetTaskState(ctx, "oneshot-past")
	if state.Status != scheduler.TaskStatusCompleted {
		t.Errorf("expected completed status, got %s", state.Status)
	}
}

func TestScheduler_OneShotTask_ValidationErrors(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage)

	ctx := t.Context()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
	if err != scheduler.ErrScheduleConflict {
		t.Errorf("expected ErrScheduleConflict, got %v", err)
	}

	// Neither RunAt nor Schedule → existing "schedule is required" error
	err = s.Register(ctx, corescheduler.TaskConfig{
		ID:   "empty",
		Func: noop,
	})
	if err == nil {
		t.Error("expected error for empty schedule and empty RunAt")
	}
}

func TestScheduler_OneShotTask_TriggerCompleted(t *testing.T) {
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

	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:    "oneshot-trigger",
		RunAt: time.Now().Add(-time.Second), // Execute immediately
		Func:  func(_ context.Context) error { return nil },
	})
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Wait for execution to complete
	time.Sleep(300 * time.Millisecond)

	// Verify completed
	state, _ := s.GetTaskState(ctx, "oneshot-trigger")
	if state.Status != scheduler.TaskStatusCompleted {
		t.Fatalf("expected completed, got %s", state.Status)
	}

	// Try to trigger completed task
	if err := s.TriggerTask(ctx, "oneshot-trigger"); err != scheduler.ErrTaskCompleted {
		t.Errorf("expected ErrTaskCompleted, got %v", err)
	}
}

func TestScheduler_OneShotTask_ManualTriggerBeforeTime(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(50*time.Millisecond))

	ctx := t.Context()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Manual trigger before scheduled time
	if err := s.TriggerTask(ctx, "oneshot-manual"); err != nil {
		t.Fatalf("failed to trigger task: %v", err)
	}

	// Wait for execution
	time.Sleep(300 * time.Millisecond)

	if execCount.Load() != 1 {
		t.Errorf("expected 1 execution, got %d", execCount.Load())
	}

	// Should be completed
	state, _ := s.GetTaskState(ctx, "oneshot-manual")
	if state.Status != scheduler.TaskStatusCompleted {
		t.Errorf("expected completed status after manual trigger, got %s", state.Status)
	}
}

func TestScheduler_OneShotTask_PauseResume(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(50*time.Millisecond))

	ctx := t.Context()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Pause before execution
	if err := s.PauseTask(ctx, "oneshot-pause"); err != nil {
		t.Fatalf("failed to pause: %v", err)
	}

	// Wait past the RunAt time
	time.Sleep(400 * time.Millisecond)
	if execCount.Load() != 0 {
		t.Error("task executed while paused")
	}

	// Resume
	if err := s.ResumeTask(ctx, "oneshot-pause"); err != nil {
		t.Fatalf("failed to resume: %v", err)
	}

	// Should execute now (RunAt was in past, resume sets NextRunAt to now)
	time.Sleep(300 * time.Millisecond)
	if execCount.Load() != 1 {
		t.Errorf("expected 1 execution after resume, got %d", execCount.Load())
	}

	// Verify completed
	state, _ := s.GetTaskState(ctx, "oneshot-pause")
	if state.Status != scheduler.TaskStatusCompleted {
		t.Errorf("expected completed, got %s", state.Status)
	}

	// Pause on completed task should fail
	if err := s.PauseTask(ctx, "oneshot-pause"); err != scheduler.ErrTaskCompleted {
		t.Errorf("expected ErrTaskCompleted on pause, got %v", err)
	}
}

func TestScheduler_OneShotTask_History(t *testing.T) {
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

	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:    "oneshot-history",
		RunAt: time.Now().Add(-time.Second), // Execute immediately
		Func:  func(_ context.Context) error { return nil },
	})
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Wait for execution
	time.Sleep(300 * time.Millisecond)

	// Verify history was recorded
	var history []*scheduler.TaskHistory
	for h, err := range s.History(ctx, "oneshot-history") {
		if err != nil {
			t.Fatalf("failed to iterate history: %v", err)
		}
		history = append(history, h)
	}

	if len(history) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(history))
	}

	if len(history) > 0 && !history[0].Success {
		t.Error("expected successful execution in history")
	}
}

func TestScheduler_OneShotTask_TaskSummary(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage)

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
		ID:    "oneshot-summary",
		RunAt: time.Now().Add(time.Hour),
		Func:  func(_ context.Context) error { return nil },
	})
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	for summary, err := range s.Tasks(ctx) {
		if err != nil {
			t.Fatalf("failed to iterate tasks: %v", err)
		}
		if summary.ID == "oneshot-summary" {
			if !summary.OneShot {
				t.Error("expected OneShot flag in TaskSummary")
			}
			if summary.Schedule != "" {
				t.Errorf("expected empty schedule for one-shot, got %q", summary.Schedule)
			}
			return
		}
	}
	t.Error("oneshot-summary not found in Tasks()")
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
		if err != nil {
			t.Fatalf("failed to upsert task: %v", err)
		}
	}

	// Test slices.Collect with iterator
	tasks := slices.Collect(tasksSeqValues(storage.Tasks(ctx)))
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}
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
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
		if err != nil {
			t.Fatalf("failed to register task %s: %v", id, err)
		}
	}

	// Wait for tasks to run
	time.Sleep(500 * time.Millisecond)

	observed := maxObserved.Load()
	if observed > 2 {
		t.Errorf("dynamic concurrent tasks exceeded limit: max observed %d, limit was 2", observed)
	}
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
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
		if err != nil {
			t.Fatalf("failed to register task %s: %v", id, err)
		}
	}

	// Phase 1: limit=1, verify max concurrent ≤ 1
	time.Sleep(500 * time.Millisecond)
	phase1Max := maxObserved.Load()
	if phase1Max > 1 {
		t.Errorf("phase 1: expected max 1 concurrent, got %d", phase1Max)
	}

	// Phase 2: increase limit to 4
	maxObserved.Store(0)
	limit.Store(4)
	time.Sleep(1500 * time.Millisecond)

	phase2Max := maxObserved.Load()
	if phase2Max > 4 {
		t.Errorf("phase 2: expected max 4 concurrent, got %d", phase2Max)
	}
	// With limit=4 and 4 tasks, we should see >1 concurrent
	if phase2Max <= 1 {
		t.Errorf("phase 2: expected >1 concurrent after limit increase, got %d", phase2Max)
	}
}

func TestScheduler_WithEnvironment(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithEnvironment(concurrency.EnvironmentRateLimited), // limit = 3
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

	// With RateLimited environment (limit=3) and no running tasks, available should be 3
	available := s.AvailableSlots()
	if available != 3 {
		t.Errorf("expected 3 available slots for RateLimited environment, got %d", available)
	}

	// RunningTasksCount should be 0 initially
	if s.RunningTasksCount() != 0 {
		t.Errorf("expected 0 running tasks initially, got %d", s.RunningTasksCount())
	}

	// AvailableHighPrioritySlots returns -1 in dynamic mode
	if s.AvailableHighPrioritySlots() != -1 {
		t.Errorf("expected -1 for AvailableHighPrioritySlots in dynamic mode, got %d", s.AvailableHighPrioritySlots())
	}
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
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
	if err != nil {
		t.Fatalf("failed to register normal task: %v", err)
	}

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
	if err != nil {
		t.Fatalf("failed to register critical task: %v", err)
	}

	// Wait for tasks to run
	time.Sleep(400 * time.Millisecond)

	if criticalCount.Load() == 0 {
		t.Error("critical task did not run in dynamic mode")
	}
	if normalCount.Load() == 0 {
		t.Error("normal task did not run in dynamic mode")
	}
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
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	// Dynamic takes precedence: available should be 3, not 10
	available := s.AvailableSlots()
	if available != 3 {
		t.Errorf("expected 3 available slots (dynamic precedence), got %d", available)
	}
}

func TestScheduler_DynamicRunningCountAccuracy(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithConcurrencyLimitFunc(func() int { return 10 }),
	)

	ctx := t.Context()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Wait for task to start
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("task did not start")
	}

	// Running count should be 1
	if count := s.RunningTasksCount(); count != 1 {
		t.Errorf("expected 1 running task, got %d", count)
	}

	// Available should be 9 (10 - 1)
	if avail := s.AvailableSlots(); avail != 9 {
		t.Errorf("expected 9 available slots, got %d", avail)
	}

	// Let the task finish
	close(done)
	time.Sleep(200 * time.Millisecond)

	// Running count should return to 0
	if count := s.RunningTasksCount(); count != 0 {
		t.Errorf("expected 0 running tasks after completion, got %d", count)
	}
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
	if err := storage.UpsertTask(ctx, staleState); err != nil {
		t.Fatalf("failed to upsert stale task: %v", err)
	}

	// Start a new scheduler — startup recovery should reset the task
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithStaleTaskTimeout(30*time.Minute),
	)
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	// Verify the task was reset to Active
	state, err := s.GetTaskState(ctx, "stale-startup")
	if err != nil {
		t.Fatalf("failed to get task state: %v", err)
	}
	if state.Status != scheduler.TaskStatusActive {
		t.Errorf("expected task to be reset to Active after startup recovery, got %s", state.Status)
	}
	if state.Failures != 1 {
		t.Errorf("expected failures to be incremented to 1, got %d", state.Failures)
	}

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
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	time.Sleep(2 * time.Second)

	if execCount.Load() == 0 {
		t.Error("recovered task should have executed after registration")
	}
}

func TestScheduler_RecoverStaleTasksPeriodic(t *testing.T) {
	storage := memory.New(100)

	// Use a very short stale timeout for testing
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithStaleTaskTimeout(200*time.Millisecond),
	)

	ctx := t.Context()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Manually set the task to Running in storage (simulating a stuck task)
	state, _ := s.GetTaskState(ctx, "stale-periodic")
	state.Status = scheduler.TaskStatusRunning
	state.UpdatedAt = time.Now().Add(-time.Minute).Unix() // Well past the 200ms timeout
	if err := storage.UpsertTask(ctx, state); err != nil {
		t.Fatalf("failed to upsert stuck task: %v", err)
	}

	// Wait for periodic recovery (ticker interval = min(200ms, 5min) = 200ms)
	time.Sleep(500 * time.Millisecond)

	// Verify the task was recovered to Active
	state, err = s.GetTaskState(ctx, "stale-periodic")
	if err != nil {
		t.Fatalf("failed to get task state: %v", err)
	}
	if state.Status == scheduler.TaskStatusRunning {
		t.Error("expected stale task to be recovered from Running status")
	}
}

func TestScheduler_RunningTaskNotRecoveredPrematurely(t *testing.T) {
	storage := memory.New(100)

	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithStaleTaskTimeout(10*time.Second),
	)

	ctx := t.Context()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}
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
	if err != nil {
		t.Fatalf("failed to register task: %v", err)
	}

	// Wait for the task to start executing
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("task did not start")
	}

	// Verify the task is in Running status
	state, _ := s.GetTaskState(ctx, "long-running")
	if state.Status != scheduler.TaskStatusRunning {
		t.Fatalf("expected task to be in Running status, got %s", state.Status)
	}

	// Wait a bit — the task should NOT be recovered since it's within the timeout
	// and actively running in this process
	time.Sleep(500 * time.Millisecond)

	state, _ = s.GetTaskState(ctx, "long-running")
	if state.Status != scheduler.TaskStatusRunning {
		t.Errorf("running task was prematurely recovered: status = %s", state.Status)
	}

	// Let the task finish
	close(done)
	time.Sleep(200 * time.Millisecond)

	state, _ = s.GetTaskState(ctx, "long-running")
	if state.Status != scheduler.TaskStatusActive {
		t.Errorf("expected task to return to Active after completion, got %s", state.Status)
	}
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
		if err != nil {
			t.Fatalf("register task %q: %v", id, err)
		}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.NextCursor == nil {
		t.Fatal("expected nextCursor for more pages")
	}

	// Second page
	result2, err := s.TasksPaginated(ctx, scheduler.PageRequest{Limit: 2, Cursor: *result.NextCursor}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result2.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result2.Items))
	}
	// IDs should not overlap
	if result.Items[0].ID == result2.Items[0].ID {
		t.Fatal("page 2 IDs should not overlap with page 1")
	}

	// Third page
	result3, err := s.TasksPaginated(ctx, scheduler.PageRequest{Limit: 2, Cursor: *result2.NextCursor}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result3.Items) != 1 {
		t.Fatalf("expected 1 item on last page, got %d", len(result3.Items))
	}
	if result3.NextCursor != nil {
		t.Fatal("expected nil nextCursor on last page")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result.Items))
	}
	if result.NextCursor != nil {
		t.Fatal("expected nil nextCursor when all items fit in one page")
	}
}

func TestScheduler_TasksPaginated_InvalidCursor(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(time.Hour))
	ctx := t.Context()
	_ = s.Start(ctx)
	defer func() { _ = s.Stop(ctx) }()

	_, err := s.TasksPaginated(ctx, scheduler.PageRequest{Cursor: "garbage"}, "")
	if err == nil {
		t.Fatal("expected error for invalid cursor")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.NextCursor == nil {
		t.Fatal("expected nextCursor for more pages")
	}
	// Should be sorted desc by StartedAt
	if result.Items[0].StartedAt < result.Items[1].StartedAt {
		t.Fatal("expected descending order by StartedAt")
	}

	// Page 2
	result2, err := s.HistoryPaginated(ctx, "hist-task", scheduler.PageRequest{Limit: 2, Cursor: *result.NextCursor}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result2.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result2.Items))
	}

	// Page 3
	result3, err := s.HistoryPaginated(ctx, "hist-task", scheduler.PageRequest{Limit: 2, Cursor: *result2.NextCursor}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result3.Items) != 1 {
		t.Fatalf("expected 1 item on last page, got %d", len(result3.Items))
	}
	if result3.NextCursor != nil {
		t.Fatal("expected nil nextCursor on last page")
	}
}

func TestScheduler_HistoryPaginated_InvalidCursor(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(time.Hour))
	ctx := t.Context()
	_ = s.Start(ctx)
	defer func() { _ = s.Stop(ctx) }()

	_, err := s.HistoryPaginated(ctx, "any-task", scheduler.PageRequest{Cursor: "bad"}, "")
	if err == nil {
		t.Fatal("expected error for invalid cursor")
	}
}

func TestScheduler_TasksPaginated_WithFilter(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(time.Hour))
	ctx := t.Context()
	_ = s.Start(ctx)
	defer func() { _ = s.Stop(ctx) }()

	// Register 5 tasks, then pause 2 of them.
	registerDummyTasks(t, s, 5)

	if err := s.PauseTask(ctx, generatePaddedID(1)); err != nil {
		t.Fatalf("pause task: %v", err)
	}
	if err := s.PauseTask(ctx, generatePaddedID(3)); err != nil {
		t.Fatalf("pause task: %v", err)
	}

	// Filter: status == 1 (Active) — should return 3 tasks.
	result, err := s.TasksPaginated(ctx, scheduler.PageRequest{Limit: 10}, "status == 1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 active tasks, got %d", len(result.Items))
	}
	for _, item := range result.Items {
		if item.Status != scheduler.TaskStatusActive {
			t.Errorf("expected active status, got %v for task %s", item.Status, item.ID)
		}
	}

	// Filter: status == 2 (Paused) — should return 2 tasks.
	result, err = s.TasksPaginated(ctx, scheduler.PageRequest{Limit: 10}, "status == 2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 paused tasks, got %d", len(result.Items))
	}
	for _, item := range result.Items {
		if item.Status != scheduler.TaskStatusPaused {
			t.Errorf("expected paused status, got %v for task %s", item.Status, item.ID)
		}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 successful entries, got %d", len(result.Items))
	}
	for _, item := range result.Items {
		if !item.Success {
			t.Errorf("expected success=true, got false for entry %s", item.ID)
		}
	}

	// Filter: success == false — should return 3 entries.
	result, err = s.HistoryPaginated(ctx, "filter-hist-task", scheduler.PageRequest{Limit: 10}, "success == false")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 failed entries, got %d", len(result.Items))
	}
	for _, item := range result.Items {
		if item.Success {
			t.Errorf("expected success=false, got true for entry %s", item.ID)
		}
	}
}
