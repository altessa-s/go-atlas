// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas-core/runtime/cycle"
	"github.com/altessa-s/go-atlas-core/types/nilcheck"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	schedtask "github.com/altessa-s/go-horai/task"
)

// ErrSchedulerManaged is returned by [Orchestrator.RunRecoveryCycle] when the
// recovery cycle has been registered with a scheduler (via WithScheduler +
// WithRecoverySchedule). In that case the scheduler owns the cadence and direct
// invocation is rejected to avoid two concurrent cycles.
var ErrSchedulerManaged = cycle.ErrManaged

// RegisterRecovery installs the configured recovery task. It may be retried
// after failure; manual recovery remains available until registration succeeds.
func (o *Orchestrator[T]) RegisterRecovery(ctx context.Context) error {
	o.registrationMu.Lock()
	defer o.registrationMu.Unlock()
	if o.recoveryTask.IsManaged() || nilcheck.IsNil(o.scheduler) || o.recoverySchedule == "" {
		return nil
	}
	ctx, cancel := corecontext.WithMaxTimeout(ctx, o.storageTimeout)
	defer cancel()
	if err := o.scheduler.Register(ctx, schedtask.Config{
		ID:             o.recoveryTaskID,
		Description:    "Resume or roll back stalled and timed-out saga instances",
		Func:           func(ctx context.Context) error { return o.recoveryTask.TryRun(ctx) },
		Schedule:       o.recoverySchedule,
		Priority:       schedtask.PriorityNormal,
		DisableHistory: true,
	}); err != nil {
		return err
	}
	o.recoveryTask.MarkManaged()
	return nil
}

// RunRecoveryCycle performs a single recovery pass: it fetches recoverable
// instances (those past their deadline, or left mid-compensation) and, for each,
// auto-rolls-back the timed-out ones and resumes the rest. It is safe to call
// concurrently; an in-progress cycle causes overlapping calls to return
// immediately. When the cycle is scheduler-managed it returns
// [ErrSchedulerManaged].
func (o *Orchestrator[T]) RunRecoveryCycle(ctx context.Context) error {
	return o.recoveryTask.Run(ctx)
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
		if o.serializer.Deserialize(inst.Data, &data) != nil {
			return o.quarantine(ctx, inst, "undecodable payload")
		}
	}
	if verr := o.validate(inst); verr != nil {
		return o.quarantine(ctx, inst, verr.Error())
	}
	_, err = o.execute(ctx, inst, &data, true)
	switch {
	case err == nil, errors.Is(err, ErrAlreadyTerminal), errors.Is(err, ErrVersionConflict), errors.Is(err, ErrInstanceBusy):
		return nil
	default:
		if _, ok := errors.AsType[*leaseAcquireError](err); ok {
			o.deferScan(ctx, inst.ID)
		}
		return err
	}
}

// quarantine moves an instance the orchestrator can never run (undecodable
// payload, cursor outside the definition) to Failed so it leaves the recovery
// scan, then fires the dead-letter hook. The write is version-checked against
// the version recovery read: a concurrent healthy writer wins and the instance
// is left alone. class is a bounded description; it never carries payload text.
func (o *Orchestrator[T]) quarantine(ctx context.Context, inst *Instance, class string) error {
	inst.Status = StatusFailed
	inst.LastError = "saga: quarantined: " + class
	inst.LeaseOwner = ""
	inst.LeaseUntil = time.Time{}
	inst.UpdatedAt = time.Now().UTC()
	if err := o.store.Update(ctx, inst); err != nil {
		if errors.Is(err, ErrVersionConflict) {
			return nil
		}
		return err
	}
	o.metrics.failed.Inc()
	o.logger.WarnContext(ctx, "saga: instance quarantined", slog.String("id", inst.ID), slog.String("class", class))
	o.fireDeadLetter(inst)
	return nil
}

// deferScan pushes the lease of an instance whose first lease write failed one
// grace period forward, so a persistently failing instance leaves the head of
// the scan instead of starving the instances behind it. It is best effort.
func (o *Orchestrator[T]) deferScan(ctx context.Context, id string) {
	cur, err := o.store.Get(ctx, id)
	if err != nil || cur.Status.IsTerminal() {
		return
	}
	cur.LeaseUntil = time.Now().UTC().Add(o.leaseGrace)
	_ = o.store.Update(ctx, cur)
}
