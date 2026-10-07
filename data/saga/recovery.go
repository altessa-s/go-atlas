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

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
)

// ErrSchedulerManaged is returned by [Orchestrator.RunRecoveryCycle] when the
// recovery cycle has been registered with a scheduler (via WithScheduler +
// WithRecoverySchedule). In that case the scheduler owns the cadence and direct
// invocation is rejected to avoid two concurrent cycles.
var ErrSchedulerManaged = corescheduler.ErrSchedulerManaged

// RegisterRecovery installs the configured recovery task. It may be retried
// after failure; manual recovery remains available until registration succeeds.
func (o *Orchestrator[T]) RegisterRecovery(ctx context.Context) error {
	o.registrationMu.Lock()
	defer o.registrationMu.Unlock()
	if o.recoveryTask.Registered() || nilcheck.IsNil(o.scheduler) || o.recoverySchedule == "" {
		return nil
	}
	ctx, cancel := corecontext.WithMaxTimeout(ctx, o.storageTimeout)
	defer cancel()
	if err := o.scheduler.Register(ctx, corescheduler.TaskConfig{
		ID:             o.recoveryTaskID,
		Description:    "Resume or roll back stalled and timed-out saga instances",
		Func:           func(ctx context.Context) error { return o.recoveryTask.TryRun(ctx, o.runRecoveryCycleInternal) },
		Schedule:       o.recoverySchedule,
		Priority:       corescheduler.TaskPriorityNormal,
		DisableHistory: true,
	}); err != nil {
		return err
	}
	o.recoveryTask.MarkRegistered()
	return nil
}

// RunRecoveryCycle performs a single recovery pass: it fetches recoverable
// instances (those past their deadline, or left mid-compensation) and, for each,
// auto-rolls-back the timed-out ones and resumes the rest. It is safe to call
// concurrently; an in-progress cycle causes overlapping calls to return
// immediately. When the cycle is scheduler-managed it returns
// [ErrSchedulerManaged].
func (o *Orchestrator[T]) RunRecoveryCycle(ctx context.Context) error {
	return o.recoveryTask.Run(ctx, o.runRecoveryCycleInternal)
}

// runRecoveryCycleInternal is the unguarded recovery pass used both by the
// public method and the scheduler task.
func (o *Orchestrator[T]) runRecoveryCycleInternal(ctx context.Context) error {
	if !nilcheck.IsNil(o.leaderElector) && !o.leaderElector.IsLeader() {
		return nil // Not the leader; another node runs the recovery cycle.
	}
	o.metrics.recoveryCycles.Inc()

	cycleCtx, cancel := corecontext.WithMaxTimeout(ctx, o.recoveryTimeout)
	defer cancel()
	now := time.Now().UTC()

	insts, err := o.store.FetchRecoverable(cycleCtx, o.def.name, now, o.recoveryBatchSize)
	if err != nil {
		return coreerrs.WrapOperation(err, "fetch recoverable saga instances")
	}
	if len(insts) == 0 {
		return nil
	}

	var failures []error
	for _, inst := range insts {
		if err := cycleCtx.Err(); err != nil {
			return err
		}
		if inst.Definition != o.def.name {
			continue // Belongs to another saga type sharing the store.
		}
		o.metrics.recovered.Inc()
		if err := o.recoverInstance(cycleCtx, inst, now); err != nil {
			failures = append(failures, err)
			o.logger.WarnContext(ctx, "saga: recovery failed for instance",
				slog.String("id", inst.ID),
				slog.String("status", string(inst.Status)),
				slog.Any("error", err),
			)
		}
	}
	return errors.Join(append(failures, cycleCtx.Err())...)
}

// recoverInstance reloads and acquires execution before considering rollback.
func (o *Orchestrator[T]) recoverInstance(ctx context.Context, candidate *Instance, _ time.Time) error {
	inst, err := o.store.Get(ctx, candidate.ID)
	if err != nil {
		return err
	}
	if inst.Status.IsTerminal() || inst.Definition != o.def.name {
		return nil
	}
	var data T
	if len(inst.Data) > 0 {
		if decodeErr := o.serializer.Deserialize(inst.Data, &data); decodeErr != nil {
			return decodeErr
		}
	}
	_, err = o.execute(ctx, inst, &data, true)
	switch {
	case err == nil, errors.Is(err, sagaerrs.ErrAlreadyTerminal), errors.Is(err, sagaerrs.ErrVersionConflict), errors.Is(err, sagaerrs.ErrInstanceBusy):
		return nil
	default:
		return err
	}
}
