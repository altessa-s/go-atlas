// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"context"
	"encoding/hex"
	"log/slog"
	"sync/atomic"

	"github.com/altessa-s/go-atlas-core/encoding/keys"
	"github.com/altessa-s/go-atlas-core/runtime/cycle"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	schedtask "github.com/altessa-s/go-horai/task"
)

// schedulerCallback lives independently of its manager. A registrar may keep
// its method value indefinitely after shutdown.
type schedulerCallback struct{ guard atomic.Pointer[cycle.Guard] }

func (c *schedulerCallback) run(ctx context.Context) error {
	if g := c.guard.Load(); g != nil {
		return g.TryRun(ctx)
	}
	return nil
}

// RegisterSchedulerFunc returns a function for use by a scheduler and marks
// this manager as scheduler-managed. After calling this method, direct calls
// to RunUpdateCycle will return [cycle.ErrManaged].
func (m *Manager) RegisterSchedulerFunc() func(context.Context) error {
	m.updateCycleTask.MarkManaged()
	return m.scheduled.run
}

// RunUpdateCycle fetches the latest policies and reloads them if changed.
// This method is designed to be called manually for one-time updates.
// If the manager is registered with a scheduler, this method returns
// [cycle.ErrManaged].
func (m *Manager) RunUpdateCycle(ctx context.Context) error {
	return m.updateCycleTask.Run(ctx)
}

// registerSchedulerTasks registers background tasks with the scheduler if configured.
func (m *Manager) registerSchedulerTasks(ctx context.Context, o *options) error {
	if o.updateSchedule == "" {
		return nil
	}

	id := o.schedulerTaskID
	if id == "" {
		const digestBytes = 12
		digest := keys.Digest([]byte(m.source.Name()), []byte(m.query))
		id = "opa-policy-update-" + hex.EncodeToString(digest[:digestBytes])
	}
	taskCfg := schedtask.Config{
		ID:          id,
		Description: "Periodic OPA policy bundle update",
		Func:        m.RegisterSchedulerFunc(),
		Schedule:    o.updateSchedule,
		RunOnStart:  o.runOnStart,
		Priority:    schedtask.PriorityNormal,
	}

	if err := o.scheduler.Register(ctx, taskCfg); err != nil {
		return coreerrs.WrapOperation(err, "register policy update task")
	}

	m.logger.Info("registered policy update task",
		slog.String("schedule", o.updateSchedule))

	return nil
}
