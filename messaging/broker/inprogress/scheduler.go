// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package inprogress

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	schedtask "github.com/altessa-s/go-horai/task"
)

// defaultTickTaskIDPrefix prefixes the scheduler task ID derived for a manager
// that has no explicit one.
const defaultTickTaskIDPrefix = "broker-inprogress-heartbeat"

// managerSeq numbers the managers of the process that derive a task ID.
var managerSeq atomic.Uint64

// registerTickTask registers the tick cycle task with the scheduler if configured.
func (m *Manager) registerTickTask(opts *options) error {
	if nilcheck.IsNil(m.scheduler) || opts.tickSchedule == "" {
		return nil
	}

	if opts.tickTaskID == "" {
		opts.tickTaskID = fmt.Sprintf(defaultTickTaskIDPrefix+"-%d", managerSeq.Add(1))
	}

	ctx := context.Background()
	taskCfg := schedtask.Config{
		ID:          opts.tickTaskID,
		Description: "Execute heartbeat tick cycle for in-progress messages",
		// TryRun rather than SchedulerFunc: the scheduler-managed flag flips
		// only after Register succeeds, so a failed registration leaves
		// RunTickCycle usable and heartbeats keep flowing.
		Func: func(ctx context.Context) error {
			return m.tickTask.TryRun(ctx)
		},
		Schedule:       opts.tickSchedule,
		Priority:       schedtask.PriorityNormal,
		DisableHistory: true,
	}

	if err := m.scheduler.Register(ctx, taskCfg); err != nil {
		return coreerrs.WrapOperation(err, "register tick cycle task")
	}
	m.tickTask.MarkManaged()

	return nil
}

// RegisterTickSchedulerFunc returns a function for use by a scheduler and marks
// tick as scheduler-managed. After calling this method, direct calls to
// RunTickCycle will return [cycle.ErrManaged].
func (m *Manager) RegisterTickSchedulerFunc() func(context.Context) error {
	return m.tickTask.Managed()
}

// RunTickCycle executes a single tick cycle for checking and sending InProgress heartbeats.
// This method is designed to be called manually for one-time tick.
// If the function is registered with a scheduler, this method returns
// [cycle.ErrManaged].
func (m *Manager) RunTickCycle(ctx context.Context) error {
	return m.tickTask.Run(ctx)
}

// runTickCycleInternal performs the actual tick cycle.
// Callers must route through tickTask so overlapping cycles collapse
// into a single execution.
func (m *Manager) runTickCycleInternal(ctx context.Context) error {
	// Check if context is already canceled
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return m.tick(ctx)
}
