// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// RegisterSchedulerFunc returns a function for use by a scheduler and marks
// this manager as scheduler-managed. After calling this method, direct calls
// to RunUpdateCycle will return ErrSchedulerManaged.
func (m *Manager) RegisterSchedulerFunc() func(context.Context) error {
	m.schedulerRegistered.Store(true)
	return m.runUpdateCycleInternal
}

// RunUpdateCycle fetches the latest policies and reloads them if changed.
// This method is designed to be called manually for one-time updates.
// If the manager is registered with a scheduler, this method returns ErrSchedulerManaged.
func (m *Manager) RunUpdateCycle(ctx context.Context) error {
	if m.schedulerRegistered.Load() {
		return ErrSchedulerManaged
	}
	return m.runUpdateCycleInternal(ctx)
}

// registerSchedulerTasks registers background tasks with the scheduler if configured.
func (m *Manager) registerSchedulerTasks(ctx context.Context, o *options) error {
	if nilcheck.IsNil(o.scheduler) {
		return nil
	}

	if o.updateSchedule == "" {
		return nil
	}

	taskCfg := corescheduler.TaskConfig{
		ID:          "opa-policy-update",
		Description: "Periodic OPA policy bundle update",
		Func:        m.RegisterSchedulerFunc(),
		Schedule:    o.updateSchedule,
		RunOnStart:  o.runOnStart,
		Priority:    corescheduler.TaskPriorityNormal,
	}

	if err := o.scheduler.Register(ctx, taskCfg); err != nil {
		return coreerrs.WrapOperation(err, "register policy update task")
	}

	m.logger.Info("registered policy update task",
		slog.String("schedule", o.updateSchedule))

	return nil
}
