// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
)

// ErrSchedulerManaged is returned by [Orchestrator.RunRecoveryCycle] when the
// recovery cycle has been registered with a scheduler (via WithScheduler +
// WithRecoverySchedule). In that case the scheduler owns the cadence and direct
// invocation is rejected to avoid two concurrent cycles.
var ErrSchedulerManaged = errors.New("saga: recovery cycle is managed by the scheduler")

// registerRecoveryTask registers the recovery cycle with the configured
// scheduler. It marks the cycle scheduler-managed so direct
// [Orchestrator.RunRecoveryCycle] calls are rejected.
func (o *Orchestrator[T]) registerRecoveryTask() error {
	if nilcheck.IsNil(o.scheduler) {
		return nil
	}
	o.schedulerRecoveryRegistered.Store(true)
	return o.scheduler.Register(o.baseCtx, corescheduler.TaskConfig{
		ID:             o.recoveryTaskID,
		Description:    "Resume or roll back stalled and timed-out saga instances",
		Func:           o.runRecoveryCycleInternal,
		Schedule:       o.recoverySchedule,
		Priority:       corescheduler.TaskPriorityNormal,
		DisableHistory: true,
	})
}

// RunRecoveryCycle performs a single recovery pass: it fetches recoverable
// instances (those past their deadline, or left mid-compensation) and, for each,
// auto-rolls-back the timed-out ones and resumes the rest. It is safe to call
// concurrently; an in-progress cycle causes overlapping calls to return
// immediately. When the cycle is scheduler-managed it returns
// [ErrSchedulerManaged].
func (o *Orchestrator[T]) RunRecoveryCycle(ctx context.Context) error {
	if o.schedulerRecoveryRegistered.Load() {
		return ErrSchedulerManaged
	}
	return o.runRecoveryCycleInternal(ctx)
}

// runRecoveryCycleInternal is the unguarded recovery pass used both by the
// public method and the scheduler task.
func (o *Orchestrator[T]) runRecoveryCycleInternal(ctx context.Context) error {
	if !nilcheck.IsNil(o.leaderElector) && !o.leaderElector.IsLeader() {
		return nil // Not the leader; another node runs the recovery cycle.
	}
	if !o.recoveryRunning.CompareAndSwap(false, true) {
		return nil // Already running; skip this cycle.
	}
	defer o.recoveryRunning.Store(false)

	o.metrics.recoveryCycles.Inc()

	// Detach from the cycle's cancellation so a single pass can finish its
	// per-instance work, bounded by step/saga timeouts rather than the tick.
	cycleCtx := context.WithoutCancel(ctx)
	now := time.Now().UTC()

	insts, err := o.store.FetchRecoverable(cycleCtx, now, o.recoveryBatchSize)
	if err != nil {
		if coreerrs.IsContextCanceled(err) {
			return nil
		}
		return coreerrs.WrapOperation(err, "fetch recoverable saga instances")
	}
	if len(insts) == 0 {
		return nil
	}

	for _, inst := range insts {
		if inst.Definition != o.def.name {
			continue // Belongs to another saga type sharing the store.
		}
		o.metrics.recovered.Inc()
		if err := o.recoverInstance(cycleCtx, inst, now); err != nil {
			o.logger.WarnContext(ctx, "saga: recovery failed for instance",
				slog.String("id", inst.ID),
				slog.String("status", string(inst.Status)),
				slog.Any("error", err),
			)
		}
	}
	return nil
}

// recoverInstance handles one recoverable instance: a Running instance past its
// deadline is flipped to Compensating (auto-rollback) before being driven;
// every other non-terminal instance is simply resumed. Version conflicts are
// benign — another coordinator owns the instance — and are ignored.
func (o *Orchestrator[T]) recoverInstance(ctx context.Context, inst *Instance, now time.Time) error {
	if inst.Status == StatusRunning && !inst.Deadline.IsZero() && !now.Before(inst.Deadline) {
		inst.Status = StatusCompensating
		inst.LastError = "saga: deadline exceeded; auto-rolled back by recovery"
		inst.UpdatedAt = now
		if err := o.store.Update(ctx, inst); err != nil {
			if errors.Is(err, sagaerrs.ErrVersionConflict) {
				return nil
			}
			return coreerrs.WrapOperation(err, "mark instance for rollback")
		}
	}

	_, err := o.Resume(ctx, inst.ID)
	switch {
	case err == nil, errors.Is(err, sagaerrs.ErrAlreadyTerminal), errors.Is(err, sagaerrs.ErrVersionConflict):
		return nil
	default:
		return err
	}
}
