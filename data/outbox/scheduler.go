// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"

	corectx "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// scheduledTask describes one outbox cycle's scheduler registration.
type scheduledTask struct {
	name        string                     // Used in the error message only.
	id          string                     // Scheduler task ID.
	description string                     // Human-facing description.
	schedule    string                     // Cron expression; empty disables registration.
	enabled     bool                       // Extra precondition beyond a non-empty schedule.
	task        *corescheduler.ManagedTask // Single-flight guard for this cycle.
	run         corescheduler.TaskFunc     // The cycle body.

	// managed exempts the task from the scheduler's pause/disable operations.
	// Cleanup is the only cycle an operator can safely stop at runtime — the
	// others would silently halt delivery — so it is the only managed one.
	managed bool
}

// registerTasks registers outbox tasks with the scheduler if configured.
//
// A cycle is flagged scheduler-managed only after its registration succeeds.
// Marking it up front (as [corescheduler.ManagedTask.SchedulerFunc] does) would
// mean a rejected registration — an invalid cron expression, say — leaves the
// cycle both unscheduled and unusable by hand, because RunXxxCycle would keep
// returning [corescheduler.ErrSchedulerManaged]. New only logs a registration
// failure, so that combination would silently disable the cycle outright.
func (o *Outbox) registerTasks(opts *options) error {
	if nilcheck.IsNil(o.scheduler) {
		return nil
	}

	if err := validateTaskIDs(opts); err != nil {
		return err
	}

	ctx := context.Background()

	tasks := [...]scheduledTask{
		{
			name:        "dispatch",
			id:          opts.dispatchTaskID,
			description: "Dispatch unprocessed events from outbox",
			schedule:    opts.dispatchSchedule,
			enabled:     true,
			task:        &o.dispatchTask,
			run:         o.runDispatchCycleInternal,
		},
		{
			name:        "unlock",
			id:          opts.unlockTaskID,
			description: "Unlock stuck events in outbox",
			schedule:    opts.unlockSchedule,
			enabled:     true,
			task:        &o.unlockTask,
			run:         o.runUnlockCycleInternal,
		},
		{
			name:        "expire",
			id:          opts.expireTaskID,
			description: "Mark expired events in outbox",
			schedule:    opts.expireSchedule,
			enabled:     opts.defaultEventTTL > 0,
			task:        &o.expireTask,
			run:         o.runExpireCycleInternal,
		},
		{
			name:        "stats",
			id:          opts.statsTaskID,
			description: "Refresh outbox backlog and dead-letter gauges",
			schedule:    opts.statsSchedule,
			enabled:     true,
			task:        &o.statsTask,
			run:         o.runStatsCycleInternal,
		},
		{
			name:        "cleanup",
			id:          opts.cleanupTaskID,
			description: "Cleanup old published events from outbox",
			schedule:    opts.cleanupSchedule,
			enabled:     opts.publishedEventsLifetime > 0,
			task:        &o.cleanupTask,
			run:         o.runCleanupCycleInternal,
			managed:     true,
		},
	}

	for _, t := range tasks {
		if t.schedule == "" || !t.enabled {
			continue
		}

		taskCfg := corescheduler.TaskConfig{
			ID:          t.id,
			Description: t.description,
			// TryRun rather than Run: the scheduler is the registered driver,
			// so it must not be turned away by the scheduler-managed check it
			// is itself the reason for.
			Func:           func(ctx context.Context) error { return t.task.TryRun(ctx, t.run) },
			Schedule:       t.schedule,
			Priority:       corescheduler.TaskPriorityNormal,
			Unmanaged:      !t.managed,
			DisableHistory: true,
		}
		if err := o.scheduler.Register(ctx, taskCfg); err != nil {
			return coreerrs.WrapOperationWithContext(err, "register outbox scheduler task", t.name)
		}
		t.task.MarkRegistered()
	}

	return nil
}

// RegisterDispatchSchedulerFunc returns a function for use by a scheduler and marks
// dispatch as scheduler-managed. After calling this method, direct calls to
// RunDispatchCycle will return [corescheduler.ErrSchedulerManaged].
func (o *Outbox) RegisterDispatchSchedulerFunc() func(context.Context) error {
	return o.dispatchTask.SchedulerFunc(o.runDispatchCycleInternal)
}

// RegisterUnlockSchedulerFunc returns a function for use by a scheduler and marks
// unlock as scheduler-managed. After calling this method, direct calls to
// RunUnlockCycle will return [corescheduler.ErrSchedulerManaged].
func (o *Outbox) RegisterUnlockSchedulerFunc() func(context.Context) error {
	return o.unlockTask.SchedulerFunc(o.runUnlockCycleInternal)
}

// RegisterCleanupSchedulerFunc returns a function for use by a scheduler and marks
// cleanup as scheduler-managed. After calling this method, direct calls to
// RunCleanupCycle will return [corescheduler.ErrSchedulerManaged].
func (o *Outbox) RegisterCleanupSchedulerFunc() func(context.Context) error {
	return o.cleanupTask.SchedulerFunc(o.runCleanupCycleInternal)
}

// RunDispatchCycle executes a single fetch-and-dispatch cycle for unprocessed events.
// This method is designed to be called manually for one-time dispatch.
// If the function is registered with a scheduler, this method returns
// [corescheduler.ErrSchedulerManaged].
func (o *Outbox) RunDispatchCycle(ctx context.Context) error {
	return o.dispatchTask.Run(ctx, o.runDispatchCycleInternal)
}

// runDispatchCycleInternal performs the actual dispatch cycle.
// Callers must route through dispatchTask so overlapping cycles collapse
// into a single execution.
func (o *Outbox) runDispatchCycleInternal(ctx context.Context) error {
	// Create a context for this processing cycle, derived from the main context
	// to allow cancellation propagation, but use WithoutCancel for the operation itself
	// to let it attempt completion, bounded by specific timeouts.
	cycleCtx := context.WithoutCancel(ctx)

	stop := o.metrics.dispatchDuration.Start()
	defer stop()

	fetchCtx, cancelFetch := corectx.ApplyTimeout(cycleCtx, o.fetchTimeout)
	events, err := o.store.FetchUnprocessedEvents(fetchCtx, o.eventsBatchSize)
	cancelFetch()

	if err != nil {
		if coreerrs.IsContextCanceled(err) {
			return nil
		}
		return coreerrs.WrapOperation(err, "fetch unprocessed events")
	}

	if len(events) == 0 {
		return nil // No events fetched
	}

	o.logger.DebugContext(ctx, "fetched unprocessed events", slog.Int("count", len(events)))

	// Create context for handleEvents with its own timeout, derived from the *main* context.
	handleCtx, handleCtxCancel := corectx.ApplyTimeout(ctx, o.handleTimeout)
	defer handleCtxCancel()

	o.handleEvents(handleCtx, events...)
	return nil
}

// RunUnlockCycle executes a single cycle to unlock stuck events in the store.
// This method is designed to be called manually for one-time unlock.
// If the function is registered with a scheduler, this method returns
// [corescheduler.ErrSchedulerManaged].
func (o *Outbox) RunUnlockCycle(ctx context.Context) error {
	return o.unlockTask.Run(ctx, o.runUnlockCycleInternal)
}

// runUnlockCycleInternal performs the actual unlock cycle.
// Callers must route through unlockTask so overlapping cycles collapse
// into a single execution.
func (o *Outbox) runUnlockCycleInternal(ctx context.Context) error {
	stop := o.metrics.unlockDuration.Start()
	defer stop()

	return o.store.UnlockStuckEvents(ctx, o.maxLockTime)
}

// RunCleanupCycle executes a single cycle to delete processed events from the store.
// This method is designed to be called manually for one-time cleanup.
// If the function is registered with a scheduler, this method returns
// [corescheduler.ErrSchedulerManaged].
func (o *Outbox) RunCleanupCycle(ctx context.Context) error {
	return o.cleanupTask.Run(ctx, o.runCleanupCycleInternal)
}

// runCleanupCycleInternal performs the actual cleanup cycle.
// Callers must route through cleanupTask so overlapping cycles collapse
// into a single execution.
func (o *Outbox) runCleanupCycleInternal(ctx context.Context) error {
	if o.publishedEventsLifetime <= 0 {
		return nil
	}

	stop := o.metrics.cleanupDuration.Start()
	defer stop()

	return o.store.DeleteProcessedEvents(ctx, o.publishedEventsLifetime)
}

// RegisterExpireSchedulerFunc returns a function for use by a scheduler and marks
// expire as scheduler-managed. After calling this method, direct calls to
// RunExpireCycle will return [corescheduler.ErrSchedulerManaged].
func (o *Outbox) RegisterExpireSchedulerFunc() func(context.Context) error {
	return o.expireTask.SchedulerFunc(o.runExpireCycleInternal)
}

// RunExpireCycle executes a single cycle to mark expired events in the store.
// This method is designed to be called manually for one-time expiration.
// If the function is registered with a scheduler, this method returns
// [corescheduler.ErrSchedulerManaged].
func (o *Outbox) RunExpireCycle(ctx context.Context) error {
	return o.expireTask.Run(ctx, o.runExpireCycleInternal)
}

// validateTaskIDs rejects configurations in which two or more of the
// scheduler task IDs (dispatch / unlock / expire / cleanup / stats) collide.
// The underlying scheduler upserts by ID, so a collision would silently
// overwrite the first task's Func pointer with the second's instead of
// running both — we'd rather fail loudly at startup than ship a partially
// scheduled outbox. Checks all IDs unconditionally (even when some
// schedules are empty, so the next operator who flips the schedule on
// inherits a working set of IDs).
func validateTaskIDs(opts *options) error {
	ids := [...]struct {
		name, value string
	}{
		{"dispatchTaskID", opts.dispatchTaskID},
		{"unlockTaskID", opts.unlockTaskID},
		{"expireTaskID", opts.expireTaskID},
		{"cleanupTaskID", opts.cleanupTaskID},
		{"statsTaskID", opts.statsTaskID},
	}
	seen := make(map[string]string, len(ids))
	for _, id := range ids {
		if id.value == "" {
			// Generated WithXxx setters TrimSpace to non-empty; the
			// defaults are non-empty constants. An empty value here
			// would mean a programmatic caller bypassed the setter and
			// assigned the field directly — not a collision but still
			// an invalid scheduler ID.
			return coreerrs.Wrapf(ErrTaskIDCollision, "%s is empty", id.name)
		}
		if prev, dup := seen[id.value]; dup {
			return coreerrs.Wrapf(ErrTaskIDCollision, "%s and %s both resolve to %q", prev, id.name, id.value)
		}
		seen[id.value] = id.name
	}
	return nil
}

// RegisterStatsSchedulerFunc returns a function for use by a scheduler and marks
// stats as scheduler-managed. After calling this method, direct calls to
// RunStatsCycle will return [corescheduler.ErrSchedulerManaged].
func (o *Outbox) RegisterStatsSchedulerFunc() func(context.Context) error {
	return o.statsTask.SchedulerFunc(o.runStatsCycleInternal)
}

// RunStatsCycle refreshes the backlog gauges from a single [Store.Stats] read.
// This method is designed to be called manually for one-time collection.
// If the function is registered with a scheduler, this method returns
// [corescheduler.ErrSchedulerManaged].
func (o *Outbox) RunStatsCycle(ctx context.Context) error {
	return o.statsTask.Run(ctx, o.runStatsCycleInternal)
}

// runStatsCycleInternal publishes the backlog snapshot as gauges so queue depth,
// dead-letter depth, and dispatch lag are alertable. Callers must route through
// statsTask so overlapping cycles collapse into a single execution.
func (o *Outbox) runStatsCycleInternal(ctx context.Context) error {
	stop := o.metrics.statsDuration.Start()
	defer stop()

	stats, err := o.store.Stats(ctx)
	if err != nil {
		return coreerrs.WrapOperation(err, "collect outbox stats")
	}

	o.metrics.pendingEvents.Set(float64(stats.Pending))
	o.metrics.inProgressEvents.Set(float64(stats.InProgress))
	o.metrics.deadLetteredEvents.Set(float64(stats.DeadLettered))
	o.metrics.oldestPendingAge.Set(stats.OldestPendingAge.Seconds())

	if stats.DeadLettered > 0 {
		o.logger.WarnContext(ctx, "outbox holds dead-lettered events awaiting operator action",
			slog.Int64("dead_lettered", stats.DeadLettered),
		)
	}

	return nil
}

// runExpireCycleInternal performs the actual expire cycle.
// It marks pending or failed events whose ExpiresAt has passed as expired.
// Callers must route through expireTask so overlapping cycles collapse
// into a single execution.
func (o *Outbox) runExpireCycleInternal(ctx context.Context) error {
	stop := o.metrics.expireDuration.Start()
	defer stop()

	count, err := o.store.ExpireEvents(ctx)
	if err != nil {
		return coreerrs.WrapOperation(err, "expire events")
	}

	if count > 0 {
		o.metrics.eventsExpired.Add(float64(count))
		o.logger.InfoContext(ctx, "expired events marked", slog.Int64("count", count))
	}

	return nil
}
