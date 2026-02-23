// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package inprogress

import (
	"context"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// registerTickTask registers the tick cycle task with the scheduler if configured.
func (m *Manager) registerTickTask(opts *options) error {
	if m.scheduler == nil || opts.tickSchedule == "" {
		return nil
	}

	ctx := context.Background()
	taskCfg := corescheduler.TaskConfig{
		ID:             "broker-inprogress-heartbeat",
		Description:    "Execute heartbeat tick cycle for in-progress messages",
		Func:           m.RegisterTickSchedulerFunc(),
		Schedule:       opts.tickSchedule,
		Priority:       corescheduler.TaskPriorityNormal,
		DisableHistory: true,
	}

	if err := m.scheduler.Register(ctx, taskCfg); err != nil {
		return coreerrs.WrapOperation(err, "register tick cycle task")
	}

	return nil
}

// RegisterTickSchedulerFunc returns a function for use by a scheduler and marks
// tick as scheduler-managed. After calling this method, direct calls to
// RunTickCycle will return ErrSchedulerManaged.
func (m *Manager) RegisterTickSchedulerFunc() func(context.Context) error {
	m.schedulerTickRegistered.Store(true)
	return m.runTickCycleInternal
}

// RunTickCycle executes a single tick cycle for checking and sending InProgress heartbeats.
// This method is designed to be called manually for one-time tick.
// If the function is registered with a scheduler, this method returns ErrSchedulerManaged.
func (m *Manager) RunTickCycle(ctx context.Context) error {
	if m.schedulerTickRegistered.Load() {
		return ErrSchedulerManaged
	}
	return m.runTickCycleInternal(ctx)
}

// runTickCycleInternal performs the actual tick cycle.
// It is safe to call concurrently; if already running, returns immediately.
func (m *Manager) runTickCycleInternal(ctx context.Context) error {
	// Prevent concurrent execution
	if !m.tickRunning.CompareAndSwap(false, true) {
		return nil // Already running, skip this cycle
	}
	defer m.tickRunning.Store(false)

	// Check if context is already canceled
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	m.tick()
	return nil
}
