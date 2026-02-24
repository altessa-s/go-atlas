// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"cmp"
	"context"
	"log/slog"
	"runtime"
	"slices"
	"time"
)

// loadTasks verifies that the storage backend is reachable and logs every
// persisted task state for diagnostics. It does not populate the in-memory
// dispatch map — tasks must be re-registered with their functions via
// [Scheduler.Register] after Start returns.
func (s *Scheduler) loadTasks(ctx context.Context) error {
	for state, err := range s.storage.Tasks(ctx) {
		if err != nil {
			return err
		}
		s.logger.DebugContext(ctx, "found persisted task state",
			slog.String("task_id", state.ID),
			slog.String("status", state.Status.String()),
			slog.String("priority", state.Priority.String()))
	}

	return nil
}

// run is the main scheduler loop.
func (s *Scheduler) run() {
	ticker := time.NewTicker(s.opts.tickInterval)
	defer ticker.Stop()

	cleanupTicker := time.NewTicker(s.opts.cleanupInterval)
	defer cleanupTicker.Stop()

	staleRecoveryInterval := min(s.opts.staleTaskTimeout, maxStaleRecoveryInterval)
	staleRecoveryTicker := time.NewTicker(staleRecoveryInterval)
	defer staleRecoveryTicker.Stop()

	for {
		select {
		case <-s.stopCtx.Done():
			return

		case <-ticker.C:
			s.tick()

		case <-cleanupTicker.C:
			s.cleanup()

		case <-staleRecoveryTicker.C:
			s.recoverStaleTasks(s.stopCtx, false)
		}
	}
}

// tick processes all tasks and executes those that are due.
// Tasks are processed in priority order: Critical > High > Normal > Low.
//
// A single Tasks() call fetches all persisted states, which are then
// cross-referenced with the in-memory registration map. This avoids
// N individual GetTask calls per tick for networked backends.
func (s *Scheduler) tick() {
	// Skip execution if not the leader
	if !s.IsLeader() {
		s.logger.DebugContext(s.stopCtx, "skipping tick: not leader")
		return
	}

	s.mu.RLock()
	tasksCopy := make(map[string]*registeredTask, len(s.tasks))
	for id, task := range s.tasks {
		tasksCopy[id] = task
	}
	s.mu.RUnlock()

	now := time.Now()

	// Fetch all persisted states in a single storage round-trip. The slice
	// is materialized before processing to avoid holding the storage iterator
	// open while performing writes (which would deadlock the memory backend).
	var allStates []*TaskState
	for state, err := range s.storage.Tasks(s.stopCtx) {
		if err != nil {
			s.logger.ErrorContext(s.stopCtx, "failed to iterate task states",
				slog.Any("error", err))
			return
		}
		allStates = append(allStates, state)
	}

	// Collect pending tasks from the fetched states.
	pending := make([]pendingTask, 0, len(tasksCopy))

	for _, state := range allStates {
		// Only consider tasks that are registered in this process.
		task, registered := tasksCopy[state.ID]
		if !registered {
			continue
		}

		// Skip if not active or already running
		if state.Status != TaskStatusActive {
			continue
		}

		if task.running.Load() {
			continue
		}

		// Check if due
		if now.Unix() < state.NextRunAt {
			continue
		}

		// Handle skip - re-read state to minimize race window with concurrent updates
		if state.SkipNextRun {
			// Re-read fresh state to avoid overwriting concurrent modifications
			freshState, err := s.storage.GetTask(s.stopCtx, state.ID)
			if err != nil || freshState == nil || !freshState.SkipNextRun {
				// State changed concurrently or error occurred, skip this update
				continue
			}
			// Apply only our changes to the fresh state
			freshState.SkipNextRun = false
			if freshState.OneShot {
				// One-shot task: skipping means completed (no next run)
				freshState.Status = TaskStatusCompleted
				freshState.NextRunAt = 0
			} else {
				freshState.NextRunAt = s.calculateNextRun(s.stopCtx, now, freshState).Unix()
			}
			freshState.UpdatedAt = now.Unix()
			if err := s.storage.UpsertTask(s.stopCtx, freshState); err != nil {
				s.logger.ErrorContext(s.stopCtx, "failed to update skipped task",
					slog.String("task_id", state.ID),
					slog.Any("error", err))
			}
			s.logger.InfoContext(s.stopCtx, "task run skipped", slog.String("task_id", state.ID))
			continue
		}

		pending = append(pending, pendingTask{task: task, state: state})
	}

	// Sort by priority (highest first) using cmp.Compare
	slices.SortFunc(pending, func(a, b pendingTask) int {
		// Higher priority value = more important, so sort descending
		return cmp.Compare(b.task.config.Priority, a.task.config.Priority)
	})

	// Dispatch pending tasks respecting concurrency limits
	s.dispatchPending(pending)
}

// dispatchPending routes pending tasks to the appropriate dispatch strategy.
func (s *Scheduler) dispatchPending(pending []pendingTask) {
	if s.opts.concurrencyLimitFunc != nil {
		s.dispatchDynamic(pending)
		return
	}
	if s.semaphore == nil {
		// Static unlimited mode: no semaphore to gate goroutines, so use
		// tick-gated dispatch via runningCount to prevent goroutine storms.
		s.dispatchUnlimited(pending)
		return
	}
	// Static limited mode: dispatch all tasks, let semaphores handle concurrency.
	for _, pt := range pending {
		s.wg.Go(func() {
			s.runTaskWithSemaphore(pt.task, pt.state)
		})
	}
}

// dispatchUnlimited dispatches tasks in static unlimited mode (no semaphore,
// no concurrencyLimitFunc). It caps the number of goroutines spawned per tick
// to runtime.GOMAXPROCS * 4 to prevent goroutine storms when many tasks become
// due simultaneously. Tasks that don't fit in this tick are deferred to the
// next one. Critical-priority tasks always bypass this cap.
func (s *Scheduler) dispatchUnlimited(pending []pendingTask) {
	perTickCap := runtime.GOMAXPROCS(0) * 4
	dispatched := 0

	for _, pt := range pending {
		// Critical bypasses all limits
		if pt.task.config.Priority == TaskPriorityCritical {
			s.wg.Go(func() {
				s.executeTask(s.stopCtx, pt.task, pt.state)
			})
			continue
		}

		if dispatched >= perTickCap {
			s.logger.DebugContext(s.stopCtx, "task deferred: per-tick dispatch cap reached",
				slog.String("task_id", pt.state.ID),
				slog.Int("cap", perTickCap))
			break
		}

		dispatched++
		s.runningCount.Add(1)
		s.wg.Go(func() {
			defer s.runningCount.Add(-1)
			s.executeTask(s.stopCtx, pt.task, pt.state)
		})
	}
}

// dispatchDynamic dispatches tasks using tick-gated concurrency control.
// It evaluates the concurrency limit function and dispatches only as many
// tasks as the current limit allows, eliminating blocked goroutines.
func (s *Scheduler) dispatchDynamic(pending []pendingTask) {
	limit := s.opts.concurrencyLimitFunc()
	if limit <= 0 {
		// Limit function returned 0 or negative: unlimited mode.
		for _, pt := range pending {
			s.runningCount.Add(1)
			s.wg.Go(func() {
				defer s.runningCount.Add(-1)
				s.executeTask(s.stopCtx, pt.task, pt.state)
			})
		}
		return
	}

	running := int(s.runningCount.Load())
	available := limit - running
	reserved := s.opts.reservedHighPrioritySlots

	for _, pt := range pending {
		priority := pt.task.config.Priority

		// Critical bypasses all limits
		if priority == TaskPriorityCritical {
			s.wg.Go(func() {
				s.executeTask(s.stopCtx, pt.task, pt.state)
			})
			continue
		}

		if available <= 0 {
			s.logger.DebugContext(s.stopCtx, "task deferred: no available slots",
				slog.String("task_id", pt.state.ID),
				slog.String("priority", pt.task.config.Priority.String()),
				slog.Int("limit", limit),
				slog.Int("running", running))
			break
		}

		// Normal and Low: don't consume reserved-for-high slots
		if priority != TaskPriorityHigh && available <= reserved {
			s.logger.DebugContext(s.stopCtx, "task deferred: only reserved slots available",
				slog.String("task_id", pt.state.ID),
				slog.String("priority", pt.task.config.Priority.String()))
			break
		}

		s.runningCount.Add(1)
		available--
		if priority == TaskPriorityHigh && reserved > 0 {
			reserved--
		}

		s.wg.Go(func() {
			defer s.runningCount.Add(-1)
			s.executeTask(s.stopCtx, pt.task, pt.state)
		})
	}
}

// runTaskWithSemaphore acquires the appropriate semaphore based on priority and executes the task.
// Used by both tick-dispatched and manually triggered tasks.
func (s *Scheduler) runTaskWithSemaphore(task *registeredTask, state *TaskState) {
	priority := task.config.Priority

	// Critical tasks bypass all limits
	if priority == TaskPriorityCritical {
		s.executeTask(s.stopCtx, task, state)
		return
	}

	// Dynamic concurrency mode: track running count.
	// Capacity gating for tick-dispatched tasks is handled in dispatchDynamic.
	// For manually triggered tasks (TriggerTask), this provides accurate counting.
	if s.opts.concurrencyLimitFunc != nil {
		s.runningCount.Add(1)
		defer s.runningCount.Add(-1)
		s.executeTask(s.stopCtx, task, state)
		return
	}

	// Static unlimited mode: track running count for observability.
	if s.semaphore == nil {
		s.runningCount.Add(1)
		defer s.runningCount.Add(-1)
		s.executeTask(s.stopCtx, task, state)
		return
	}

	// High priority: try reserved slots first, then shared pool
	if priority == TaskPriorityHigh && s.highPrioritySemaphore != nil {
		select {
		case s.highPrioritySemaphore <- struct{}{}:
			// Got reserved slot
			s.highPrioritySemaphoreUsed.Add(1)
			defer func() {
				<-s.highPrioritySemaphore
				s.highPrioritySemaphoreUsed.Add(-1)
			}()
			s.executeTask(s.stopCtx, task, state)
			return
		default:
			// Reserved slots full, try shared pool
		}
	}

	// Normal/Low priority or High that couldn't get reserved slot: use shared pool
	// Low priority: only run if there are available slots (non-blocking check first)
	if priority == TaskPriorityLow {
		select {
		case s.semaphore <- struct{}{}:
			// Got slot
			s.semaphoreUsed.Add(1)
			defer func() {
				<-s.semaphore
				s.semaphoreUsed.Add(-1)
			}()
			s.executeTask(s.stopCtx, task, state)
		default:
			// No slots available, Low priority task will wait for next tick
			s.logger.DebugContext(s.stopCtx, "low priority task deferred due to no available slots",
				slog.String("task_id", state.ID))
		}
		return
	}

	// Normal and High (fallback): wait for slot
	select {
	case s.semaphore <- struct{}{}:
		// Got slot
		s.semaphoreUsed.Add(1)
		defer func() {
			<-s.semaphore
			s.semaphoreUsed.Add(-1)
		}()
		s.executeTask(s.stopCtx, task, state)
	case <-s.stopCtx.Done():
		// Scheduler is stopping
		return
	}
}

// executeTask runs a single task and records the result.
func (s *Scheduler) executeTask(ctx context.Context, task *registeredTask, state *TaskState) {
	if !task.running.CompareAndSwap(false, true) {
		return // Already running
	}
	defer task.running.Store(false)

	runID := generateID()
	startTime := time.Now()

	s.logger.DebugContext(ctx, "task execution started",
		slog.String("task_id", state.ID),
		slog.String("priority", task.config.Priority.String()),
		slog.String("run_id", runID))

	// Re-read fresh state and update to running to avoid overwriting concurrent changes
	currentState, err := s.storage.GetTask(ctx, state.ID)
	if err != nil || currentState == nil {
		s.logger.ErrorContext(ctx, "failed to get current task state",
			slog.String("task_id", state.ID),
			slog.Any("error", err))
		return
	}
	// Only proceed if task is still active (not paused/disabled during scheduling)
	if currentState.Status != TaskStatusActive {
		s.logger.DebugContext(ctx, "task status changed, skipping execution",
			slog.String("task_id", state.ID),
			slog.String("current_status", currentState.Status.String()))
		return
	}
	currentState.Status = TaskStatusRunning
	currentState.RunStartedAt = startTime.Unix()
	currentState.UpdatedAt = startTime.Unix()
	if upsertErr := s.storage.UpsertTask(ctx, currentState); upsertErr != nil {
		s.logger.ErrorContext(ctx, "failed to update task status to running, aborting execution",
			slog.String("task_id", state.ID),
			slog.Any("error", upsertErr))
		return
	}
	// Use currentState for the rest of execution.
	//
	// NOTE: there is a narrow race window between GetTask and UpsertTask above
	// where another process could modify the state. True compare-and-swap (CAS)
	// semantics would require storage-level support (e.g. MongoDB findAndModify
	// with a version field). The re-read pattern used here minimizes but does
	// not eliminate this window.
	state = currentState

	// Create execution context with timeout if specified
	execCtx := ctx
	var cancel context.CancelFunc
	if task.config.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, task.config.Timeout)
		defer cancel()
	}

	// Execute the task
	execErr := task.config.Func(execCtx)

	endTime := time.Now()
	duration := endTime.Sub(startTime)
	success := execErr == nil

	// Record history unless disabled
	if !state.DisableHistory {
		history := &TaskHistory{
			ID:         generateID(),
			TaskID:     state.ID,
			RunID:      runID,
			StartedAt:  startTime.Unix(),
			EndedAt:    endTime.Unix(),
			DurationMs: duration.Milliseconds(),
			Success:    success,
		}

		if execErr != nil {
			history.Error = execErr.Error()
		}

		if histErr := s.storage.AddHistory(ctx, history); histErr != nil {
			s.logger.ErrorContext(ctx, "failed to record task history",
				slog.String("task_id", state.ID),
				slog.Any("error", histErr))
		}
	}

	// Re-read fresh state to minimize race with concurrent updates (e.g., Pause, SetInterval)
	freshState, err := s.storage.GetTask(ctx, state.ID)
	if err != nil || freshState == nil {
		s.logger.ErrorContext(ctx, "failed to get fresh task state after execution",
			slog.String("task_id", state.ID),
			slog.Any("error", err))
		return
	}

	// Apply execution results to fresh state, preserving concurrent modifications
	if freshState.OneShot {
		// One-shot task: mark as completed, no next run
		if freshState.Status == TaskStatusRunning {
			freshState.Status = TaskStatusCompleted
		}
		freshState.NextRunAt = 0
	} else {
		// Only set to Active if the task wasn't paused/disabled during execution
		if freshState.Status == TaskStatusRunning {
			freshState.Status = TaskStatusActive
		}
		freshState.NextRunAt = s.calculateNextRun(ctx, startTime, freshState).Unix()
	}
	freshState.LastRunAt = startTime.Unix()
	freshState.LastRunID = runID
	freshState.RunStartedAt = 0 // Clear: task is no longer running
	freshState.UpdatedAt = endTime.Unix()

	if success {
		freshState.Failures = 0
	} else {
		freshState.Failures++
		s.logger.ErrorContext(ctx, "task execution failed",
			slog.String("task_id", state.ID),
			slog.String("run_id", runID),
			slog.Duration("duration", duration),
			slog.Any("error", execErr))
	}

	if err := s.storage.UpsertTask(ctx, freshState); err != nil {
		s.logger.ErrorContext(ctx, "failed to update task state",
			slog.String("task_id", state.ID),
			slog.Any("error", err))
	}

	s.logger.DebugContext(ctx, "task execution completed",
		slog.String("task_id", state.ID),
		slog.String("run_id", runID),
		slog.Duration("duration", duration),
		slog.Bool("success", success))
}

// calculateNextRun computes the next run time based on the cron schedule.
func (s *Scheduler) calculateNextRun(ctx context.Context, from time.Time, state *TaskState) time.Time {
	sched, err := s.parser.Parse(state.Schedule)
	if err != nil {
		// This shouldn't happen since we validate schedule at registration,
		// but handle gracefully with a 1 hour fallback.
		s.logger.ErrorContext(ctx, "failed to parse cron schedule during next run calculation",
			slog.String("task_id", state.ID),
			slog.String("schedule", state.Schedule),
			slog.Any("error", err))
		return from.Add(time.Hour)
	}
	return sched.Next(from)
}

// cleanup removes old history entries.
func (s *Scheduler) cleanup() {
	if err := s.storage.CleanupHistory(s.stopCtx, s.opts.historyRetention); err != nil {
		s.logger.ErrorContext(s.stopCtx, "failed to cleanup history", slog.Any("error", err))
	}
}

// recoverStaleTasks resets tasks stuck in Running status back to Active.
// On startup (startup=true), all Running tasks are reset unconditionally since
// no goroutines are executing yet. During periodic recovery (startup=false),
// only tasks that are not currently executing in this process and have exceeded
// the stale task timeout are reset.
//
// One-shot tasks are also recovered: their NextRunAt is set to now so they
// re-execute on the next tick. This is intentional — a one-shot task found in
// Running status after a crash never completed successfully, so it should be
// retried. If this is undesirable for a particular task, callers should use
// idempotency checks inside the task function.
func (s *Scheduler) recoverStaleTasks(ctx context.Context, startup bool) {
	now := time.Now()
	timeoutSec := int64(s.opts.staleTaskTimeout.Seconds())

	// Collect stale task IDs first to avoid holding the storage iterator lock
	// while performing upserts (which would deadlock on memory storage).
	staleIDs := make([]string, 0)

	for state, err := range s.storage.Tasks(ctx) {
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to iterate tasks for stale recovery",
				slog.Any("error", err))
			return
		}

		if state.Status != TaskStatusRunning {
			continue
		}

		if !startup {
			// Use RunStartedAt for accurate elapsed time since the task entered
			// Running status. Fall back to UpdatedAt for backward compatibility
			// with states persisted before RunStartedAt was introduced.
			since := state.RunStartedAt
			if since == 0 {
				since = state.UpdatedAt
			}
			elapsed := now.Unix() - since

			// Check if the task is currently executing in this process
			s.mu.RLock()
			task, registered := s.tasks[state.ID]
			s.mu.RUnlock()

			if registered && task.running.Load() {
				// Task is actively running in this process — only recover if it
				// exceeds the stale timeout (safety net for truly stuck goroutines)
				if elapsed < timeoutSec {
					continue
				}
			} else if elapsed < timeoutSec {
				// Not running locally: check timeout
				continue
			}
		}

		staleIDs = append(staleIDs, state.ID)
	}

	// Now reset each stale task outside the iterator
	for _, id := range staleIDs {
		state, err := s.storage.GetTask(ctx, id)
		if err != nil || state == nil {
			s.logger.ErrorContext(ctx, "failed to get stale task for recovery",
				slog.String("task_id", id),
				slog.Any("error", err))
			continue
		}

		// Re-check status in case it changed during iteration
		if state.Status != TaskStatusRunning {
			continue
		}

		since := state.RunStartedAt
		if since == 0 {
			since = state.UpdatedAt
		}
		staleDuration := time.Duration(now.Unix()-since) * time.Second

		state.Status = TaskStatusActive
		state.RunStartedAt = 0
		state.Failures++
		if !state.OneShot && state.Schedule != "" {
			state.NextRunAt = s.calculateNextRun(ctx, now, state).Unix()
		} else if state.OneShot {
			state.NextRunAt = now.Unix()
		}
		state.UpdatedAt = now.Unix()

		if err := s.storage.UpsertTask(ctx, state); err != nil {
			s.logger.ErrorContext(ctx, "failed to reset stale task",
				slog.String("task_id", id),
				slog.Any("error", err))
			continue
		}

		if startup {
			s.logger.WarnContext(ctx, "recovered stale task on startup",
				slog.String("task_id", id))
		} else {
			s.logger.WarnContext(ctx, "recovered stale task",
				slog.String("task_id", id),
				slog.Duration("stale_duration", staleDuration))
		}
	}
}
