// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"context"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// registerTasks registers recovery tasks with the scheduler if configured.
func (m *Manager) registerTasks(opts *options) error {
	if nilcheck.IsNil(m.scheduler) {
		return nil
	}

	ctx := context.Background()

	// Register health check task
	if opts.healthCheckSchedule != "" {
		taskCfg := corescheduler.TaskConfig{
			ID:          "broker-recovery-health-check",
			Description: "Health check for recovery manager (fallback detection for missed advisory events)",
			Func:        m.RegisterHealthCheckSchedulerFunc(),
			Schedule:    opts.healthCheckSchedule,
			Priority:    corescheduler.TaskPriorityNormal,
		}
		if err := m.scheduler.Register(ctx, taskCfg); err != nil {
			return coreerrs.WrapOperation(err, "register health check task")
		}
	}

	// Register stale recovery cleanup task
	if opts.staleRecoveryCleanupSchedule != "" {
		taskCfg := corescheduler.TaskConfig{
			ID:          "broker-recovery-stale-cleanup",
			Description: "Cleanup stale recovery marks (clears stuck recovery marks)",
			Func:        m.RegisterStaleCleanupSchedulerFunc(),
			Schedule:    opts.staleRecoveryCleanupSchedule,
			Priority:    corescheduler.TaskPriorityNormal,
		}
		if err := m.scheduler.Register(ctx, taskCfg); err != nil {
			return coreerrs.WrapOperation(err, "register stale cleanup task")
		}
	}

	return nil
}

// RegisterHealthCheckSchedulerFunc returns a function for use by a scheduler and marks
// health check as scheduler-managed. After calling this method, direct calls to
// RunHealthCheckCycle will return [corescheduler.ErrSchedulerManaged].
func (m *Manager) RegisterHealthCheckSchedulerFunc() func(context.Context) error {
	return m.healthCheckTask.SchedulerFunc(m.runHealthCheckCycleInternal)
}

// RunHealthCheckCycle executes a single health check cycle.
// This method is designed to be called manually for one-time health check.
// If the function is registered with a scheduler, this method returns
// [corescheduler.ErrSchedulerManaged].
func (m *Manager) RunHealthCheckCycle(ctx context.Context) error {
	return m.healthCheckTask.Run(ctx, m.runHealthCheckCycleInternal)
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
// RunStaleRecoveryCleanup will return [corescheduler.ErrSchedulerManaged].
func (m *Manager) RegisterStaleCleanupSchedulerFunc() func(context.Context) error {
	return m.staleCleanupTask.SchedulerFunc(m.runStaleRecoveryCleanupInternal)
}

// RunStaleRecoveryCleanup executes a single stale recovery cleanup cycle.
// This method is designed to be called manually for one-time cleanup.
// If the function is registered with a scheduler, this method returns
// [corescheduler.ErrSchedulerManaged].
func (m *Manager) RunStaleRecoveryCleanup(ctx context.Context) error {
	return m.staleCleanupTask.Run(ctx, m.runStaleRecoveryCleanupInternal)
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
