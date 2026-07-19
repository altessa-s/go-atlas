// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package scheduler defines the core interface and value types for task
// scheduling. It acts as a dependency-inversion boundary: subsystems that need
// to register periodic or one-shot tasks accept a [TaskRegistrar] without
// importing the concrete scheduler implementation in service/scheduler.
//
// # Registering Tasks
//
// Pass a [TaskConfig] to [TaskRegistrar.Register] with at least an ID, a
// [TaskFunc], and either a cron Schedule or a one-shot RunAt time:
//
//	err := registrar.Register(ctx, scheduler.TaskConfig{
//	    ID:       "refresh-cache",
//	    Schedule: "@every 5m",
//	    Func: func(ctx context.Context) error {
//	        return refreshCache(ctx)
//	    },
//	    Priority: scheduler.TaskPriorityNormal,
//	    Timeout:  30 * time.Second,
//	})
//
// # Priority Model
//
// [TaskPriority] controls dispatch order when concurrency slots are contended:
//   - [TaskPriorityLow] -- runs only when no higher-priority tasks are waiting
//   - [TaskPriorityNormal] -- the default for most workloads
//   - [TaskPriorityHigh] -- has reserved slots and preempts Normal/Low
//   - [TaskPriorityCritical] -- bypasses concurrency limits entirely
//
// # Scheduler-Managed Cycles
//
// Subsystems that expose both a scheduler-registered task and a manual RunXxx
// entry point embed a [ManagedTask] per cycle. It owns the single-flight
// guard (overlapping executions collapse to a no-op) and the
// scheduler-managed flag: once the cycle is handed to a scheduler via
// [ManagedTask.SchedulerFunc], manual [ManagedTask.Run] calls return
// [ErrSchedulerManaged]:
//
//	type Worker struct {
//	    cycleTask scheduler.ManagedTask
//	}
//
//	func (w *Worker) RegisterSchedulerFunc() func(context.Context) error {
//	    return w.cycleTask.SchedulerFunc(w.runCycle)
//	}
//
//	func (w *Worker) RunCycle(ctx context.Context) error {
//	    return w.cycleTask.Run(ctx, w.runCycle)
//	}
//
// All types in this package are safe for concurrent use.
package scheduler
