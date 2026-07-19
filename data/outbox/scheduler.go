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

// registerTasks registers outbox tasks with the scheduler if configured.
func (o *Outbox) registerTasks(opts *options) error {
	if nilcheck.IsNil(o.scheduler) {
		return nil
	}

	if err := validateTaskIDs(opts); err != nil {
		return err
	}

	ctx := context.Background()

	// Register dispatch task
	if opts.dispatchSchedule != "" {
		taskCfg := corescheduler.TaskConfig{
			ID:             opts.dispatchTaskID,
			Description:    "Dispatch unprocessed events from outbox",
			Func:           o.RegisterDispatchSchedulerFunc(),
			Schedule:       opts.dispatchSchedule,
			Priority:       corescheduler.TaskPriorityNormal,
			Unmanaged:      true,
			DisableHistory: true,
		}
		if err := o.scheduler.Register(ctx, taskCfg); err != nil {
			return coreerrs.WrapOperation(err, "register dispatch task")
		}
	}

	// Register unlock task
	if opts.unlockSchedule != "" {
		taskCfg := corescheduler.TaskConfig{
			ID:             opts.unlockTaskID,
			Description:    "Unlock stuck events in outbox",
			Func:           o.RegisterUnlockSchedulerFunc(),
			Schedule:       opts.unlockSchedule,
			Priority:       corescheduler.TaskPriorityNormal,
			Unmanaged:      true,
			DisableHistory: true,
		}
		if err := o.scheduler.Register(ctx, taskCfg); err != nil {
			return coreerrs.WrapOperation(err, "register unlock task")
		}
	}

	// Register expire task if event expiration is configured
	if opts.expireSchedule != "" && opts.defaultEventTTL > 0 {
		taskCfg := corescheduler.TaskConfig{
			ID:          opts.expireTaskID,
			Description: "Mark expired events in outbox",
			Func: func(ctx context.Context) error {
				return o.expireTask.TryRun(ctx, o.runExpireCycleInternal)
			},
			Schedule:       opts.expireSchedule,
			Priority:       corescheduler.TaskPriorityNormal,
			Unmanaged:      true,
			DisableHistory: true,
		}
		if err := o.scheduler.Register(ctx, taskCfg); err != nil {
			return coreerrs.WrapOperation(err, "register expire task")
		}
		// Mark scheduler-managed only after successful registration so a
		// failed registration keeps manual RunExpireCycle usable.
		o.expireTask.MarkRegistered()
	}

	// Register cleanup task if published events lifetime is set
	if opts.cleanupSchedule != "" && opts.publishedEventsLifetime > 0 {
		taskCfg := corescheduler.TaskConfig{
			ID:             opts.cleanupTaskID,
			Description:    "Cleanup old published events from outbox",
			Func:           o.RegisterCleanupSchedulerFunc(),
			Schedule:       opts.cleanupSchedule,
			Priority:       corescheduler.TaskPriorityNormal,
			DisableHistory: true,
		}
		if err := o.scheduler.Register(ctx, taskCfg); err != nil {
			return coreerrs.WrapOperation(err, "register cleanup task")
		}
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
	events, err := o.store.FetchUnprocessedEvents(fetchCtx, o.eventsBatchSize, o.retryInterval)
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
// scheduler task IDs (dispatch / unlock / expire / cleanup) collide.
// The underlying scheduler upserts by ID, so a collision would silently
// overwrite the first task's Func pointer with the second's instead of
// running both — we'd rather fail loudly at startup than ship a partially
// scheduled outbox. Checks all four IDs unconditionally (even when some
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
