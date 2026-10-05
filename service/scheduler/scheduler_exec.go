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
	"strings"
	"sync"
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

		// A state moved to another occurrence by a different registration is
		// not this registration's to run; filtering it here keeps it from
		// taking concurrency slots on every tick. executeTask re-checks.
		if !task.matches(state) {
			continue
		}

		// An earlier run is still unfinished — the task was paused and resumed
		// while it ran — so ClaimRun would reject it; do not let it take a slot.
		if state.RunStartedAt != 0 {
			continue
		}

		if state.SkipNextRun {
			s.applySkip(now, state.ID, FenceOf(state))
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
// a concurrent Pause or schedule change is not overwritten, and the write is
// fenced on due, the state observed by the due scan: a task paused or claimed
// in between is left alone.
func (s *Scheduler) applySkip(now time.Time, id string, due TaskFence) {
	ctx, cancel := s.storageCtx(s.stopCtx)
	defer cancel()

	freshState, err := s.storage.GetTask(ctx, id)
	if err != nil || freshState == nil || !freshState.SkipNextRun || FenceOf(freshState) != due {
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

	replaced, err := s.storage.ReplaceTaskIf(ctx, freshState, due)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to update skipped task",
			slog.String("task_id", id),
			slog.Any("error", err))
	}
	if !replaced {
		// A concurrent claim, pause or schedule change won; it owns the state.
		return
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

	if !task.matches(state) {
		s.logger.DebugContext(ctx, "task state belongs to another registration, skipping execution",
			slog.String("task_id", state.ID))
		return
	}

	s.metrics.tasksRunning.Inc()
	defer s.metrics.tasksRunning.Dec()

	taskLabels := metrics.Labels{"task_id": state.ID, "priority": task.config.Priority.String()}
	s.metrics.tasksDispatched.WithLabels(taskLabels).Inc()
	stopTimer := s.metrics.taskDuration.WithLabels(taskLabels).Start()
	defer stopTimer()

	runID := s.opts.instanceID + runIDSeparator + generateID()
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
	// win, so the task body runs at most once per occurrence. state.NextRunAt and
	// state.RunAt fence the occurrence. A lost claim (already running, advanced, or no longer
	// active) is a clean no-op for this caller.
	//
	// The run is registered as live before the claim can make it visible, so
	// stale recovery in this process never mistakes it for an abandoned run.
	s.liveRuns.Store(runID, struct{}{})
	defer s.liveRuns.Delete(runID)
	claimCtx, claimCancel := s.storageCtx(ctx)
	claimed, err := s.storage.ClaimRun(claimCtx, state.ID, RunClaim{
		NextRunAt: state.NextRunAt, RunAt: state.RunAt,
		StartedAt: startTime.Unix(), RunID: runID, LeaseUntil: startTime.Unix() + leaseSeconds(s.runLease()),
	})
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
	// The claim stored the run's first lease. Keep it alive until the result is
	// recorded: deferred, so it covers the bookkeeping below and stops on every
	// return path. A claim slow enough to need its lease renewed at once must
	// confirm the run is still ours before the body starts.
	stopHeartbeat, owned := s.startRunHeartbeat(ctx, state.ID, runID, startTime)
	defer stopHeartbeat()
	if !owned {
		s.logger.WarnContext(ctx, "task run ownership not confirmed after a slow claim, skipping execution",
			slog.String("task_id", state.ID),
			slog.String("run_id", runID))
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

	nextRunAt := int64(0)
	if !freshState.OneShot {
		nextRunAt = s.calculateNextRun(recCtx, startTime, freshState).Unix()
	}
	if !success {
		s.metrics.taskErrors.WithLabels(metrics.Labels{"task_id": state.ID}).Inc()
		s.logger.ErrorContext(recCtx, "task execution failed", slog.String("task_id", state.ID), slog.Any("error", execErr))
	}
	finished, err := s.storage.FinishRun(recCtx, state.ID, runID, RunResult{
		StartedAt: startTime.Unix(), EndedAt: endTime.Unix(), NextRunAt: nextRunAt,
		RunAt: state.RunAt, Schedule: freshState.Schedule, Success: success,
	})
	if err != nil {
		s.logger.ErrorContext(recCtx, "failed to finish task run", slog.String("task_id", state.ID), slog.Any("error", err))
		return
	}
	if !finished {
		s.logger.WarnContext(recCtx, "task run no longer owned, discarding result", slog.String("task_id", state.ID), slog.String("run_id", runID))
		return
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

// runIDSeparator separates the owning instance ID from the random part of a
// run ID. The random part is hex, so the owner is everything before the last
// separator.
const runIDSeparator = "/"

// runOwner returns the instance ID that created runID, or "" for a run ID
// written before run IDs carried their owner.
func runOwner(runID string) string {
	i := strings.LastIndex(runID, runIDSeparator)
	if i < 0 {
		return ""
	}
	return runID[:i]
}

// runLease returns the lease this instance grants its own runs: the stale task
// timeout, floored at [minRunLease].
func (s *Scheduler) runLease() time.Duration {
	return max(s.opts.staleTaskTimeout, minRunLease)
}

// leaseSeconds converts a lease to whole seconds, rounding up so a stored
// lease never ends before the duration it stands for.
func leaseSeconds(lease time.Duration) int64 {
	sec := int64(lease / time.Second)
	if lease%time.Second != 0 {
		sec++ // without adding to lease, which overflows near the maximum duration
	}
	return sec
}

// startRunHeartbeat keeps the lease of run runID of task id alive until the
// returned stop function is called; stop also waits for an in-flight renewal.
// leaseBase is the instant the stored lease was computed from (ClaimRun stored
// leaseBase + lease). Each renewal is issued a third of a lease after the
// previous lease write, so it has two thirds of a lease to land before the
// stored lease runs out.
//
// When the claim itself took a third of a lease or more, the lease is renewed
// synchronously before returning, and owned reports whether that confirmed the
// run is still this instance's: a slow claim response may arrive after the
// lease expired and another instance reclaimed the task, in which case the
// caller must not run the task body. [confirmRunOwnership] decides what counts
// as confirmation.
//
// Renewals outlive cancellation of ctx for the same reason the result
// bookkeeping does: a run still finishing during Stop must not look abandoned
// to other instances. Renewal ends early once the storage reports that the run
// is no longer owned; a failed renewal is retried shortly after.
func (s *Scheduler) startRunHeartbeat(ctx context.Context, id, runID string, leaseBase time.Time) (stop func(), owned bool) {
	lease := s.runLease()
	interval := lease / 3 //nolint:mnd // renew three times per lease
	leaseSec := leaseSeconds(lease)
	hbCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	renew := func() (time.Time, bool, error) { return s.renewRunLease(hbCtx, id, runID, leaseSec) }

	leaseBase, owned = confirmRunOwnership(renew, leaseBase, interval)
	if !owned {
		cancel()
		return func() {}, false
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		timer := time.NewTimer(time.Until(leaseBase.Add(interval)))
		defer timer.Stop()
		for {
			select {
			case <-hbCtx.Done():
				return
			case <-timer.C:
			}
			base, renewed, err := renew()
			switch {
			case hbCtx.Err() != nil:
				return
			case err != nil:
				timer.Reset(min(interval, time.Second)) // transient: retry soon
			case !renewed:
				// Expected when the run finished just before stop; a run that
				// lost ownership while executing is reported when its result is
				// discarded.
				s.logger.DebugContext(hbCtx, "task run no longer owned, stopping lease renewal",
					slog.String("task_id", id),
					slog.String("run_id", runID))
				return
			default:
				leaseBase = base
				timer.Reset(time.Until(leaseBase.Add(interval)))
			}
		}
	})

	return func() {
		cancel()
		wg.Wait()
	}, true
}

// renewRunLease issues one [Storage.RenewRun] for run runID of task id, moving
// its lease to leaseSec seconds from now. It returns the instant the renewal
// was issued — the base of the new lease — and whether the run was still
// owned. Failures are counted and logged unless ctx has been canceled.
func (s *Scheduler) renewRunLease(ctx context.Context, id, runID string, leaseSec int64) (issued time.Time, renewed bool, err error) {
	opCtx, opCancel := s.storageCtx(ctx)
	defer opCancel()
	issued = time.Now()
	renewed, err = s.storage.RenewRun(opCtx, id, runID, issued.Unix()+leaseSec)
	if err != nil && ctx.Err() == nil {
		s.metrics.storageErrors.WithLabels(metrics.Labels{"op": "renew_run"}).Inc()
		s.logger.ErrorContext(ctx, "failed to renew task run lease",
			slog.String("task_id", id),
			slog.String("run_id", runID),
			slog.Any("error", err))
	}
	return issued, renewed, err
}

// maxOwnershipConfirmations bounds the synchronous renewals a slow claim may
// take to confirm, with a timely response, that its run is still owned.
const maxOwnershipConfirmations = 3

// confirmRunOwnership is the slow-claim check of [Scheduler.startRunHeartbeat].
// While a third of a lease (interval) or more has passed since leaseBase, the
// stored lease is too short to start the task body on: it renews, and accepts
// only a renewal whose response is itself fresh — one that arrives within
// interval of being issued still leaves two thirds of the new lease. A late
// response proves nothing about the present, so it is retried, up to
// maxOwnershipConfirmations attempts. It returns the base of the latest lease
// write and whether ownership is confirmed; a failed or rejected renewal means
// it is not.
func confirmRunOwnership(renew func() (time.Time, bool, error), leaseBase time.Time, interval time.Duration) (time.Time, bool) {
	for attempt := 0; time.Since(leaseBase) >= interval; attempt++ {
		base, renewed, err := renew()
		if err != nil || !renewed {
			return leaseBase, false
		}
		if attempt == maxOwnershipConfirmations-1 && time.Since(base) >= interval {
			return leaseBase, false
		}
		leaseBase = base
	}
	return leaseBase, true
}

// runAbandoned reports whether state carries an unfinished run that stale
// recovery may reset at now (Unix seconds). A run of this instance is abandoned
// as soon as it is no longer executing here. Any other run is abandoned only
// once its lease has expired. The lease the owner persisted with the claim and
// its renewals is authoritative, so instances with different stale timeouts
// agree; only a run without one — claimed by a release without leases — falls
// back to its start plus this instance's own lease.
func (s *Scheduler) runAbandoned(state *TaskState, now, leaseSec int64) bool {
	if state.RunStartedAt == 0 && state.Status != TaskStatusRunning {
		return false
	}
	if state.LastRunID != "" && runOwner(state.LastRunID) == s.opts.instanceID {
		_, live := s.liveRuns.Load(state.LastRunID)
		return !live
	}
	if state.RunLeaseUntil != 0 {
		return now > state.RunLeaseUntil
	}
	since := state.RunStartedAt
	if since == 0 {
		// States persisted before RunStartedAt was introduced.
		since = state.UpdatedAt
	}
	return now > since+leaseSec
}

// recoverStaleTasks resets abandoned runs (see [Scheduler.runAbandoned]): those
// of this instance that are no longer executing — after a restart with a stable
// [WithInstanceID], or when recording a result failed — and those of any
// instance whose lease has expired. Runs still held by a live instance are left
// alone, at startup as well as periodically, so recovery cannot hand a live run
// to a second executor. startup only selects the log message.
//
// One-shot tasks are also recovered: their NextRunAt is set to now — or kept,
// when the task was re-registered for a later RunAt during the run — so they
// re-execute on the next tick. This is intentional — a one-shot task whose run
// was abandoned never completed successfully, so it should be retried. If this
// is undesirable for a particular task, callers should use idempotency checks
// inside the task function.
func (s *Scheduler) recoverStaleTasks(ctx context.Context, startup bool) {
	now := time.Now()
	leaseSec := leaseSeconds(s.runLease())

	// States are materialized up front so the reset pass below never writes
	// while the storage iterator is open (which would deadlock the memory
	// backend), and so each write gets its own storage deadline.
	allStates, err := s.fetchStates(ctx, 0)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to iterate tasks for stale recovery",
			slog.Any("error", err))
		return
	}

	stale := make([]*TaskState, 0)
	for _, state := range allStates {
		if s.runAbandoned(state, now.Unix(), leaseSec) {
			stale = append(stale, state)
		}
	}

	// Now reset each stale task outside the iterator
	for _, state := range stale {
		s.resetStaleTask(ctx, state.ID, FenceOf(state), now, startup)
	}
}

// resetStaleTask clears the abandoned run of a single task. The state is
// re-read under its own storage deadline and written with
// [Storage.ReplaceTaskIf] fenced on the run observed as stale — revision
// included — so a run that finished, renewed its lease or was re-claimed since
// the collection pass is never overwritten. A task still in
// [TaskStatusRunning] returns to [TaskStatusActive]; one that management paused,
// disabled or resumed during the run keeps that status and its next run time.
func (s *Scheduler) resetStaleTask(ctx context.Context, id string, stale TaskFence, now time.Time, startup bool) {
	opCtx, cancel := s.storageCtx(ctx)
	defer cancel()

	state, err := s.storage.GetTask(opCtx, id)
	if err != nil || state == nil {
		s.logger.ErrorContext(opCtx, "failed to get stale task for recovery",
			slog.String("task_id", id),
			slog.Any("error", err))
		return
	}

	// The run observed as stale has finished, renewed or been replaced.
	if FenceOf(state) != stale {
		return
	}

	since := state.RunStartedAt
	if since == 0 {
		since = state.UpdatedAt
	}
	staleDuration := time.Duration(now.Unix()-since) * time.Second

	if state.Status == TaskStatusRunning {
		state.Status = TaskStatusActive
		if !state.OneShot && state.Schedule != "" {
			state.NextRunAt = s.calculateNextRun(opCtx, now, state).Unix()
		} else if state.OneShot {
			// Retry the abandoned occurrence now — unless the task was
			// re-registered for a later one during the run, which keeps its time.
			state.NextRunAt = max(state.NextRunAt, now.Unix())
		}
	}
	state.RunStartedAt = 0
	state.RunLeaseUntil = 0
	state.Failures++
	state.UpdatedAt = now.Unix()

	replaced, err := s.storage.ReplaceTaskIf(opCtx, state, stale)
	if err != nil {
		s.logger.ErrorContext(opCtx, "failed to reset stale task",
			slog.String("task_id", id),
			slog.Any("error", err))
		return
	}
	if !replaced {
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
