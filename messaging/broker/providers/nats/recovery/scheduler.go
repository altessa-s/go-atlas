// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"cmp"
	"context"
	"fmt"
	"sync/atomic"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	schedtask "github.com/altessa-s/go-horai/task"
)

// managerSeq numbers the managers of the process that register scheduler tasks.
var managerSeq atomic.Uint64

// registerTasks registers recovery tasks with the scheduler if configured.
func (m *Manager) registerTasks(opts *options) error {
	if nilcheck.IsNil(m.scheduler) {
		return nil
	}

	ctx := context.Background()

	// A manager without explicit IDs derives them from one instance number,
	// so two managers never share a scheduler task.
	instance := managerSeq.Add(1)
	healthID := cmp.Or(opts.healthCheckTaskID, fmt.Sprintf("broker-recovery-health-check-%d", instance))
	staleID := cmp.Or(opts.staleCleanupTaskID, fmt.Sprintf("broker-recovery-stale-cleanup-%d", instance))

	// Register health check task
	if opts.healthCheckSchedule != "" {
		taskCfg := schedtask.Config{
			ID:          healthID,
			Description: "Health check for recovery manager (fallback detection for missed advisory events)",
			Func:        m.RegisterHealthCheckSchedulerFunc(),
			Schedule:    opts.healthCheckSchedule,
			Priority:    schedtask.PriorityNormal,
		}
		if err := m.scheduler.Register(ctx, taskCfg); err != nil {
			return coreerrs.WrapOperation(err, "register health check task")
		}
	}

	// Register stale recovery cleanup task
	if opts.staleRecoveryCleanupSchedule != "" {
		taskCfg := schedtask.Config{
			ID:          staleID,
			Description: "Cleanup stale recovery marks (clears stuck recovery marks)",
			Func:        m.RegisterStaleCleanupSchedulerFunc(),
			Schedule:    opts.staleRecoveryCleanupSchedule,
			Priority:    schedtask.PriorityNormal,
		}
		if err := m.scheduler.Register(ctx, taskCfg); err != nil {
			return coreerrs.WrapOperation(err, "register stale cleanup task")
		}
	}

	return nil
}

// RegisterHealthCheckSchedulerFunc returns a function for use by a scheduler and marks
// health check as scheduler-managed. After calling this method, direct calls to
// RunHealthCheckCycle will return [cycle.ErrManaged].
func (m *Manager) RegisterHealthCheckSchedulerFunc() func(context.Context) error {
	return m.healthCheckTask.Managed()
}

// RunHealthCheckCycle executes a single health check cycle.
// This method is designed to be called manually for one-time health check.
// If the function is registered with a scheduler, this method returns
// [cycle.ErrManaged].
func (m *Manager) RunHealthCheckCycle(ctx context.Context) error {
	return m.healthCheckTask.Run(ctx)
}

// runHealthCheckCycleInternal performs the actual health check cycle.
// Callers must route through healthCheckTask so overlapping cycles collapse
// into a single execution.
func (m *Manager) runHealthCheckCycleInternal(ctx context.Context) error {
	if m.closed.Load() {
		return ErrManagerClosed
	}
	return m.healthMon.Run(ctx)
}

// RegisterStaleCleanupSchedulerFunc returns a function for use by a scheduler and marks
// stale cleanup as scheduler-managed. After calling this method, direct calls to
// RunStaleRecoveryCleanup will return [cycle.ErrManaged].
func (m *Manager) RegisterStaleCleanupSchedulerFunc() func(context.Context) error {
	return m.staleCleanupTask.Managed()
}

// RunStaleRecoveryCleanup executes a single stale recovery cleanup cycle.
// This method is designed to be called manually for one-time cleanup.
// If the function is registered with a scheduler, this method returns
// [cycle.ErrManaged].
func (m *Manager) RunStaleRecoveryCleanup(ctx context.Context) error {
	return m.staleCleanupTask.Run(ctx)
}

// runStaleRecoveryCleanupInternal performs the actual stale recovery cleanup cycle.
// Callers must route through staleCleanupTask so overlapping cycles collapse
// into a single execution.
func (m *Manager) runStaleRecoveryCleanupInternal(ctx context.Context) error {
	if m.closed.Load() {
		return ErrManagerClosed
	}
	return m.timeoutMon.Run(ctx)
}
