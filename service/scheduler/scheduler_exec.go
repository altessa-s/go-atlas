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

	"github.com/robfig/cron/v3"

	"github.com/altessa-s/go-atlas/observability/metrics"

	corectx "github.com/altessa-s/go-atlas/core/context"
)

// storageCtx caps a scheduler-owned [Storage] call at [WithStorageTimeout].
// The base context keeps its own cancellation — so [Scheduler.Stop] still
// aborts in-flight calls — while gaining a hard per-operation deadline that
// the scheduler's lifecycle context does not provide on its own.
func (s *Scheduler) storageCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return corectx.WithMaxTimeout(ctx, s.opts.storageTimeout)
}

// loadTasks verifies that the storage backend is reachable and logs every
// persisted task state for diagnostics. It does not populate the in-memory
// dispatch map — tasks must be re-registered with their functions via
// [Scheduler.Register] after Start returns.
func (s *Scheduler) loadTasks(ctx context.Context) error {
	opCtx, cancel := s.storageCtx(ctx)
	defer cancel()

	for state, err := range s.storage.Tasks(opCtx) {
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
	stop := s.metrics.tickDuration.Start()
	defer stop()

	// Skip execution if not the leader
	if !s.IsLeader() {
		s.logger.DebugContext(s.stopCtx, "skipping tick: not leader")
		return
	}

	// Skip execution if subsystems are not ready
	if !s.IsReady() {
		s.logger.DebugContext(s.stopCtx, "skipping tick: subsystems not ready")
		return
	}

	s.mu.RLock()
	tasksCopy := make(map[string]*registeredTask, len(s.tasks))
	for id, task := range s.tasks {
		tasksCopy[id] = task
	}
	s.mu.RUnlock()

	now := time.Now()

	dueStates, err := s.fetchDueStates(now.Unix(), len(tasksCopy))
	if err != nil {
		s.logger.ErrorContext(s.stopCtx, "failed to iterate due task states",
			slog.Any("error", err))
		return
	}

	// Collect pending tasks from the fetched states. Storage has already
	// filtered on status and due time, so only process-local conditions are
	// re-checked here.
	pending := make([]pendingTask, 0, len(dueStates))

	for _, state := range dueStates {
		// Only consider tasks that are registered in this process.
		task, registered := tasksCopy[state.ID]
		if !registered {
			continue
		}

		// Skip if a dispatch for this task is already queued or running. This
		// covers the window before the goroutine reaches executeTask, which is
		// unbounded in static semaphore mode where it blocks on the semaphore.
		if task.dispatched.Load() {
			continue
		}

		// Backstop the storage's due-time filter. ClaimRun's CAS re-checks
		// status, so a stale status cannot cause a spurious run — but nothing
		// downstream re-checks NextRunAt, so an implementation that got the
		// bound wrong would fire tasks early and silently.
		if now.Unix() < state.NextRunAt {
			continue
		}

		if state.SkipNextRun {
			s.applySkip(now, state.ID)
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

// fetchStates materializes every persisted task state in a single storage
// round-trip, bounded by [WithStorageTimeout]. The slice is materialized before
// the caller processes it so the tick never holds the storage iterator open
// while performing writes (which would deadlock the memory backend).
func (s *Scheduler) fetchStates(ctx context.Context, capacity int) ([]*TaskState, error) {
	opCtx, cancel := s.storageCtx(ctx)
	defer cancel()

	states := make([]*TaskState, 0, capacity)
	for state, err := range s.storage.Tasks(opCtx) {
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}

// fetchDueStates materializes the task states eligible for dispatch at now in
// a single storage round-trip, bounded by [WithStorageTimeout]. The filter is
// pushed down to the backend, so the tick's cost tracks the number of tasks
// actually due rather than the size of the whole collection.
func (s *Scheduler) fetchDueStates(now int64, capacity int) ([]*TaskState, error) {
	opCtx, cancel := s.storageCtx(s.stopCtx)
	defer cancel()

	states := make([]*TaskState, 0, capacity)
	for state, err := range s.storage.DueTasks(opCtx, now) {
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}

// applySkip consumes the SkipNextRun flag of a due task, advancing it to its
// following occurrence without invoking the task function. One-shot tasks are
// transitioned to [TaskStatusCompleted] instead. The state is re-read first so
// a concurrent Pause or schedule change is not overwritten.
func (s *Scheduler) applySkip(now time.Time, id string) {
	ctx, cancel := s.storageCtx(s.stopCtx)
	defer cancel()

	freshState, err := s.storage.GetTask(ctx, id)
	if err != nil || freshState == nil || !freshState.SkipNextRun {
		// State changed concurrently or the read failed: leave it alone.
		return
	}

	freshState.SkipNextRun = false
	if freshState.OneShot {
		// One-shot task: skipping means completed (no next run).
		freshState.Status = TaskStatusCompleted
		freshState.NextRunAt = 0
	} else {
		freshState.NextRunAt = s.calculateNextRun(ctx, now, freshState).Unix()
	}
	freshState.UpdatedAt = now.Unix()

	if err := s.storage.UpsertTask(ctx, freshState); err != nil {
		s.logger.ErrorContext(ctx, "failed to update skipped task",
			slog.String("task_id", id),
			slog.Any("error", err))
	}
	s.metrics.tasksSkipped.WithLabels(metrics.Labels{"task_id": id}).Inc()
	s.logger.InfoContext(ctx, "task run skipped", slog.String("task_id", id))
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
	// claim bounds the blocked set at one goroutine per registered task — the
	// semaphore acquire inside runTaskWithSemaphore blocks, so without it every
	// tick would stack another waiter onto the same still-queued task.
	for _, pt := range pending {
		if !pt.task.claim() {
			continue
		}
		s.wg.Go(func() {
			defer pt.task.release()
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
	const perTickMultiplier = 4
	perTickCap := runtime.GOMAXPROCS(0) * perTickMultiplier
	dispatched := 0

	for _, pt := range pending {
		if !pt.task.claim() {
			continue
		}

		// Critical bypasses all limits
		if pt.task.config.Priority == TaskPriorityCritical {
			s.wg.Go(func() {
				defer pt.task.release()
				s.executeTask(s.stopCtx, pt.task, pt.state)
			})
			continue
		}

		if dispatched >= perTickCap {
			pt.task.release()
			s.logger.DebugContext(s.stopCtx, "task deferred: per-tick dispatch cap reached",
				slog.String("task_id", pt.state.ID),
				slog.Int("cap", perTickCap))
			break
		}

		dispatched++
		s.runningCount.Add(1)
		s.wg.Go(func() {
			defer pt.task.release()
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
			if !pt.task.claim() {
				continue
			}
			s.runningCount.Add(1)
			s.wg.Go(func() {
				defer pt.task.release()
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

		if !pt.task.claim() {
			continue
		}

		// Critical bypasses all limits
		if priority == TaskPriorityCritical {
			s.wg.Go(func() {
				defer pt.task.release()
				s.executeTask(s.stopCtx, pt.task, pt.state)
			})
			continue
		}

		if available <= 0 {
			pt.task.release()
			s.logger.DebugContext(s.stopCtx, "task deferred: no available slots",
				slog.String("task_id", pt.state.ID),
				slog.String("priority", pt.task.config.Priority.String()),
				slog.Int("limit", limit),
				slog.Int("running", running))
			break
		}

		// Normal and Low: don't consume reserved-for-high slots
		if priority != TaskPriorityHigh && available <= reserved {
			pt.task.release()
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
			defer pt.task.release()
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

	s.metrics.tasksRunning.Inc()
	defer s.metrics.tasksRunning.Dec()

	taskLabels := metrics.Labels{"task_id": state.ID, "priority": task.config.Priority.String()}
	s.metrics.tasksDispatched.WithLabels(taskLabels).Inc()
	stopTimer := s.metrics.taskDuration.WithLabels(taskLabels).Start()
	defer stopTimer()

	runID := generateID()
	startTime := time.Now()

	// Record dispatch lag: delay between scheduled time and actual execution
	if state.NextRunAt > 0 {
		lag := startTime.Sub(time.Unix(state.NextRunAt, 0))
		if lag > 0 {
			s.metrics.dispatchLag.WithLabels(metrics.Labels{"task_id": state.ID}).ObserveDuration(lag)
		}
	}

	s.logger.DebugContext(ctx, "task execution started",
		slog.String("task_id", state.ID),
		slog.String("priority", task.config.Priority.String()),
		slog.String("run_id", runID))

	// Atomically claim this occurrence (active→running) at the storage layer.
	// The claim — not leadership — is the correctness boundary: even if two
	// scheduler instances dispatch the same occurrence (e.g. during a
	// leader-election split-brain window), the CAS in ClaimRun lets exactly one
	// win, so the task body runs at most once per occurrence. state.NextRunAt is
	// the occurrence fence. A lost claim (already running, advanced, or no longer
	// active) is a clean no-op for this caller.
	claimCtx, claimCancel := s.storageCtx(ctx)
	claimed, err := s.storage.ClaimRun(claimCtx, state.ID, state.NextRunAt, startTime.Unix(), runID)
	claimCancel()
	if err != nil {
		s.metrics.storageErrors.WithLabels(metrics.Labels{"op": "claim_run"}).Inc()
		s.logger.ErrorContext(ctx, "failed to claim task run, aborting execution",
			slog.String("task_id", state.ID),
			slog.Any("error", err))
		return
	}
	if !claimed {
		s.logger.DebugContext(ctx, "task run not claimed (already running, advanced, or not active), skipping execution",
			slog.String("task_id", state.ID))
		return
	}
	// Reflect the claimed transition locally for the remainder of execution.
	state.Status = TaskStatusRunning
	state.RunStartedAt = startTime.Unix()
	state.LastRunID = runID
	state.UpdatedAt = startTime.Unix()

	// Create execution context with timeout if specified
	execCtx, cancel := corectx.ApplyTimeout(ctx, task.config.Timeout)
	defer cancel()

	// Execute the task
	execErr := task.config.Func(execCtx)

	endTime := time.Now()
	duration := endTime.Sub(startTime)
	success := execErr == nil

	// The bookkeeping below must outlive cancellation of ctx. Stop cancels the
	// lifecycle context and only then waits on the WaitGroup, so if these
	// writes inherited that cancellation every graceful shutdown would leave
	// its in-flight tasks stuck in TaskStatusRunning, to be resurrected later
	// by stale recovery with a bogus failure count. WithoutCancel detaches
	// them; the storage timeout keeps them bounded, and Stop still waits for
	// them because the goroutine has not returned yet.
	recCtx, recCancel := s.storageCtx(context.WithoutCancel(ctx))
	defer recCancel()

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

		if histErr := s.storage.AddHistory(recCtx, history); histErr != nil {
			s.logger.ErrorContext(recCtx, "failed to record task history",
				slog.String("task_id", state.ID),
				slog.Any("error", histErr))
		}
	}

	// Re-read fresh state to minimize race with concurrent updates (e.g., Pause, SetInterval)
	freshState, err := s.storage.GetTask(recCtx, state.ID)
	if err != nil || freshState == nil {
		s.logger.ErrorContext(recCtx, "failed to get fresh task state after execution",
			slog.String("task_id", state.ID),
			slog.Any("error", err))
		return
	}

	// Fence on the run ID stamped by ClaimRun. If it no longer names our run,
	// this occurrence was taken over while we were executing — stale recovery
	// reset the task and another claim won it. Writing our result now would
	// stamp the live run as finished (status back to active, RunStartedAt
	// cleared), letting a third dispatch claim it alongside the one still in
	// flight. The history entry above is per-run and stays; only the shared
	// state is off limits.
	if freshState.LastRunID != runID {
		s.logger.WarnContext(recCtx, "task state was reclaimed during execution, discarding result",
			slog.String("task_id", state.ID),
			slog.String("run_id", runID),
			slog.String("current_run_id", freshState.LastRunID))
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
		freshState.NextRunAt = s.calculateNextRun(recCtx, startTime, freshState).Unix()
	}
	freshState.LastRunAt = startTime.Unix()
	freshState.LastRunID = runID
	freshState.RunStartedAt = 0 // Clear: task is no longer running
	freshState.UpdatedAt = endTime.Unix()

	if success {
		freshState.Failures = 0
	} else {
		freshState.Failures++
		s.metrics.taskErrors.WithLabels(metrics.Labels{"task_id": state.ID}).Inc()
		s.logger.ErrorContext(recCtx, "task execution failed",
			slog.String("task_id", state.ID),
			slog.String("run_id", runID),
			slog.Duration("duration", duration),
			slog.Any("error", execErr))
	}

	if err := s.storage.UpsertTask(recCtx, freshState); err != nil {
		s.logger.ErrorContext(recCtx, "failed to update task state",
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
	// Fast path: use cached schedule (populated during Register).
	if cached, ok := s.scheduleCache.Load(state.Schedule); ok {
		sched, _ := cached.(cron.Schedule) //nolint:errcheck // type is guaranteed by scheduleCache.Store
		return sched.Next(from)
	}

	// Slow path: parse and cache for recovered/unregistered tasks.
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
	s.scheduleCache.Store(state.Schedule, sched)
	return sched.Next(from)
}

// cleanup removes old history entries.
func (s *Scheduler) cleanup() {
	ctx, cancel := s.storageCtx(s.stopCtx)
	defer cancel()

	if err := s.storage.CleanupHistory(ctx, s.opts.historyRetention); err != nil {
		s.logger.ErrorContext(ctx, "failed to cleanup history", slog.Any("error", err))
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

	// States are materialized up front so the reset pass below never writes
	// while the storage iterator is open (which would deadlock the memory
	// backend), and so each write gets its own storage deadline.
	allStates, err := s.fetchStates(ctx, 0)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to iterate tasks for stale recovery",
			slog.Any("error", err))
		return
	}

	staleIDs := make([]string, 0)

	for _, state := range allStates {
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
		s.resetStaleTask(ctx, id, now, startup)
	}
}

// resetStaleTask returns a single task stuck in [TaskStatusRunning] to
// [TaskStatusActive]. The state is re-read under its own storage deadline
// because it may have changed since the collection pass.
func (s *Scheduler) resetStaleTask(ctx context.Context, id string, now time.Time, startup bool) {
	opCtx, cancel := s.storageCtx(ctx)
	defer cancel()

	state, err := s.storage.GetTask(opCtx, id)
	if err != nil || state == nil {
		s.logger.ErrorContext(opCtx, "failed to get stale task for recovery",
			slog.String("task_id", id),
			slog.Any("error", err))
		return
	}

	// Re-check status in case it changed during iteration
	if state.Status != TaskStatusRunning {
		return
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
		state.NextRunAt = s.calculateNextRun(opCtx, now, state).Unix()
	} else if state.OneShot {
		state.NextRunAt = now.Unix()
	}
	state.UpdatedAt = now.Unix()

	if err := s.storage.UpsertTask(opCtx, state); err != nil {
		s.logger.ErrorContext(opCtx, "failed to reset stale task",
			slog.String("task_id", id),
			slog.Any("error", err))
		return
	}

	s.metrics.staleTasksRecovered.Inc()

	if startup {
		s.logger.WarnContext(opCtx, "recovered stale task on startup",
			slog.String("task_id", id))
	} else {
		s.logger.WarnContext(opCtx, "recovered stale task",
			slog.String("task_id", id),
			slog.Duration("stale_duration", staleDuration))
	}
}
