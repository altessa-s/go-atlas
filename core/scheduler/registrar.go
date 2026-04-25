// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"context"
	"time"
)

// TaskRegistrar is the interface accepted by subsystems that need to register
// scheduled or one-shot tasks without importing the concrete scheduler
// implementation. It is satisfied by *service/scheduler.Scheduler.
//
// Implementations must be safe for concurrent use by multiple goroutines.
type TaskRegistrar interface {
	// Register adds a task described by cfg to the scheduler. It returns an
	// error if the configuration is invalid (e.g. missing [TaskConfig.ID] or
	// missing both Schedule and RunAt). The context may carry deadlines or
	// cancellation that bound the registration call itself.
	Register(ctx context.Context, cfg TaskConfig) error
}

// TaskFunc is the function executed by the scheduler when a task fires.
// The provided context carries the task's timeout (if [TaskConfig.Timeout] is
// non-zero) and is canceled when the scheduler shuts down. Returning a
// non-nil error marks the execution as failed in the task history.
type TaskFunc func(ctx context.Context) error

// TaskPriority defines the relative execution priority of a scheduled task.
// Higher-priority tasks are dispatched before lower-priority ones when the
// scheduler's concurrency slots are contended. The numeric values are aligned
// with the proto enum scheduler.v1.TaskPriority so they can be used directly
// in gRPC messages.
type TaskPriority int32

const (
	// TaskPriorityUnspecified is the zero value and must not be used when
	// registering tasks. It exists only to detect unset fields.
	TaskPriorityUnspecified TaskPriority = 0
	// TaskPriorityLow marks tasks that execute only when no
	// [TaskPriorityNormal] or [TaskPriorityHigh] tasks are waiting.
	TaskPriorityLow TaskPriority = 1
	// TaskPriorityNormal is the default priority applied to tasks that do not
	// specify an explicit [TaskConfig.Priority].
	TaskPriorityNormal TaskPriority = 2
	// TaskPriorityHigh marks tasks that have reserved execution slots and are
	// dispatched before [TaskPriorityNormal] and [TaskPriorityLow] tasks.
	TaskPriorityHigh TaskPriority = 3
	// TaskPriorityCritical marks tasks that bypass the scheduler's concurrency
	// limits entirely and execute immediately regardless of slot availability.
	TaskPriorityCritical TaskPriority = 4
)

// String returns a lowercase label for the priority (e.g. "normal", "high").
// Unrecognized values, including [TaskPriorityUnspecified], return "unknown".
func (p TaskPriority) String() string {
	switch p {
	case TaskPriorityLow:
		return "low"
	case TaskPriorityNormal:
		return "normal"
	case TaskPriorityHigh:
		return "high"
	case TaskPriorityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// TaskConfig holds all parameters needed to register a task with a
// [TaskRegistrar]. Callers must set at least ID, Func, and one of Schedule
// or RunAt. Schedule and RunAt are mutually exclusive; setting both is an
// error.
type TaskConfig struct {
	// ID is a unique, non-empty identifier for the task. Registering a
	// second task with the same ID replaces the previous registration.
	ID string
	// Description is an optional human-readable summary shown in management
	// UIs and task-history records.
	Description string
	// Schedule is a cron-style expression for periodic execution.
	// It supports standard six-field cron syntax (with seconds) and
	// convenience descriptors:
	//   - Standard: "0 */5 * * * *" (every 5 minutes)
	//   - Descriptors: "@every 5m", "@hourly", "@daily", "@weekly", "@monthly"
	//
	// Mutually exclusive with RunAt.
	Schedule string
	// Func is the [TaskFunc] invoked each time the task fires. Must not be nil.
	Func TaskFunc
	// Priority determines execution order when concurrency slots are
	// contended. Defaults to [TaskPriorityNormal] when left at zero.
	Priority TaskPriority
	// Timeout is the maximum duration allowed for a single execution of
	// Func. The scheduler derives a child context with this deadline. A zero
	// value means no timeout is applied.
	Timeout time.Duration
	// RunOnStart causes the scheduler to execute Func once immediately upon
	// registration, in addition to the normal schedule.
	RunOnStart bool
	// DisableHistory prevents the scheduler from recording execution-history
	// entries for this task, which can be useful for high-frequency
	// housekeeping tasks.
	DisableHistory bool
	// Unmanaged marks the task as exempt from runtime management operations
	// such as PauseTask and DisableTask.
	Unmanaged bool
	// RunAt specifies a single point in time at which the task should
	// execute. After execution the task is considered complete. Mutually
	// exclusive with Schedule.
	RunAt time.Time
	// Meta holds arbitrary key-value pairs attached to the task for
	// use by monitoring, logging, or management tooling.
	Meta map[string]string
}
