// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga

import (
	"context"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	schedtask "github.com/altessa-s/go-horai/task"
)

// RegisterRetention installs the retention task that deletes terminal
// instances older than the [WithRetention] window. It does nothing when
// retention is off or no scheduler is set, may be retried after failure, and
// leaves [Orchestrator.RunRetentionCycle] available until registration
// succeeds.
func (o *Orchestrator[T]) RegisterRetention(ctx context.Context) error {
	o.registrationMu.Lock()
	defer o.registrationMu.Unlock()
	if o.retention <= 0 || o.retentionTask.IsManaged() || nilcheck.IsNil(o.scheduler) {
		return nil
	}
	ctx, cancel := corecontext.WithMaxTimeout(ctx, o.storageTimeout)
	defer cancel()
	if err := o.scheduler.Register(ctx, schedtask.Config{
		ID:             RetentionTaskIDPrefix + o.def.name,
		Description:    "Delete terminal saga instances past the retention window",
		Func:           func(ctx context.Context) error { return o.retentionTask.TryRun(ctx) },
		Schedule:       o.retentionSchedule,
		Priority:       schedtask.PriorityLow,
		DisableHistory: true,
	}); err != nil {
		return err
	}
	o.retentionTask.MarkManaged()
	return nil
}

// RunRetentionCycle deletes the terminal instances of the bound definition
// whose last update is older than the [WithRetention] window, in batches until
// none are left. It does nothing when retention is off. Overlapping calls
// return immediately; when the cycle is scheduler-managed it returns
// [ErrSchedulerManaged].
func (o *Orchestrator[T]) RunRetentionCycle(ctx context.Context) error {
	return o.retentionTask.Run(ctx)
}

// runRetentionCycleInternal is the unguarded retention pass used by both the
// public method and the scheduler task.
func (o *Orchestrator[T]) runRetentionCycleInternal(ctx context.Context) error {
	if o.retention <= 0 {
		return nil
	}
	if !nilcheck.IsNil(o.leaderElector) && !o.leaderElector.IsLeader() {
		return nil // Not the leader; another node runs the retention cycle.
	}

	cycleCtx, cancel := corecontext.WithMaxTimeout(ctx, o.recoveryTimeout)
	defer cancel()
	cutoff := time.Now().UTC().Add(-o.retention)

	total := 0
	for {
		n, err := o.store.DeleteTerminal(cycleCtx, o.def.name, cutoff, o.retentionBatchSize)
		total += n
		o.metrics.retained.Add(float64(n))
		if err != nil {
			return coreerrs.WrapOperation(err, "delete terminal saga instances")
		}
		if n < o.retentionBatchSize {
			break
		}
		if err := cycleCtx.Err(); err != nil {
			return err
		}
	}
	if total > 0 {
		o.logger.InfoContext(ctx, "saga: retention deleted terminal instances",
			slog.String("definition", o.def.name), slog.Int("deleted", total))
	}
	return nil
}
