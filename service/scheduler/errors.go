// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import "errors"

// ErrTaskNotFound is returned by [Scheduler.PauseTask], [Scheduler.ResumeTask],
// [Scheduler.DisableTask], [Scheduler.EnableTask], [Scheduler.SkipNextRun],
// and [Scheduler.TriggerTask] when no task with the given ID exists in storage.
var ErrTaskNotFound = errors.New("task not found")

// ErrTaskNotRegistered is returned by [Scheduler.Unregister] and [Scheduler.TriggerTask]
// when the task ID is not present in the in-memory registration map. This differs from
// [ErrTaskNotFound], which checks storage.
var ErrTaskNotRegistered = errors.New("task not registered")

// ErrTaskUnmanaged is returned by [Scheduler.PauseTask] and [Scheduler.DisableTask]
// when the target task has its Unmanaged flag set, indicating it cannot be
// controlled through runtime management operations.
var ErrTaskUnmanaged = errors.New("task is unmanaged")

// ErrTaskNotPaused is returned by [Scheduler.ResumeTask] when the task's current
// status is not [TaskStatusPaused].
var ErrTaskNotPaused = errors.New("task is not paused")

// ErrTaskNotDisabled is returned by [Scheduler.EnableTask] when the task's current
// status is not [TaskStatusDisabled].
var ErrTaskNotDisabled = errors.New("task is not disabled")

// ErrTaskDisabled is returned by [Scheduler.TriggerTask] when the task's current
// status is [TaskStatusDisabled].
var ErrTaskDisabled = errors.New("task is disabled")

// ErrTaskCompleted is returned by [Scheduler.PauseTask], [Scheduler.ResumeTask],
// [Scheduler.EnableTask], and [Scheduler.TriggerTask] when the target task has
// [TaskStatusCompleted], which indicates a one-shot task that has already executed.
var ErrTaskCompleted = errors.New("task is completed")

// ErrNotReady is returned by [Scheduler.TriggerTask] when a [WithReadinessProbe]
// is configured and it returns false, indicating that subsystems are not yet ready.
var ErrNotReady = errors.New("subsystems not ready")

// ErrScheduleConflict is returned by [Scheduler.Register] when the provided
// [core/scheduler.TaskConfig] sets both RunAt and Schedule, which are mutually exclusive.
var ErrScheduleConflict = errors.New("RunAt and Schedule are mutually exclusive")
