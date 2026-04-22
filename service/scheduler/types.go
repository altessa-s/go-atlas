// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"iter"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/data/filter"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// Node is an alias for [filter.Node] from data/filter.
type Node = filter.Node

// UnixNow returns the current wall-clock time as Unix seconds (UTC).
// It is used internally for timestamping [TaskState] mutations.
func UnixNow() int64 { return time.Now().Unix() }

// UnixMs converts a [time.Duration] to its integer millisecond representation.
// It is a convenience helper for populating [TaskHistory.DurationMs].
func UnixMs(d time.Duration) int64 { return d.Milliseconds() }

// TaskStatus represents the lifecycle status of a scheduled task. The numeric
// values are aligned with the proto enum scheduler.v1.TaskStatus so they can be
// used directly in gRPC messages without conversion.
type TaskStatus int32

const (
	// TaskStatusUnspecified is the zero value. It should not be used when
	// creating or updating tasks; its presence signals an uninitialized field.
	TaskStatusUnspecified TaskStatus = 0
	// TaskStatusActive indicates the task is scheduled and will be dispatched
	// when its next run time arrives. This is the normal steady-state for
	// periodic tasks.
	TaskStatusActive TaskStatus = 1
	// TaskStatusPaused indicates the task has been paused via
	// [Scheduler.PauseTask] and will not execute until [Scheduler.ResumeTask]
	// is called.
	TaskStatusPaused TaskStatus = 2
	// TaskStatusDisabled indicates the task has been disabled via
	// [Scheduler.DisableTask]. Disabled tasks cannot be triggered manually
	// either. Use [Scheduler.EnableTask] to re-activate.
	TaskStatusDisabled TaskStatus = 3
	// TaskStatusRunning indicates the task's function is currently executing.
	// The scheduler transitions a task to this status just before invoking its
	// function and resets it to [TaskStatusActive] (or [TaskStatusCompleted]
	// for one-shot tasks) upon completion.
	TaskStatusRunning TaskStatus = 4
	// TaskStatusCompleted indicates a one-shot task that has finished its
	// single execution. Completed tasks are inert and reject most management
	// operations with [ErrTaskCompleted].
	TaskStatusCompleted TaskStatus = 5
)

// String returns a lowercase label for the status (e.g. "active", "paused").
// Unrecognized values, including [TaskStatusUnspecified], return "unspecified".
func (s TaskStatus) String() string {
	switch s {
	case TaskStatusActive:
		return "active"
	case TaskStatusPaused:
		return "paused"
	case TaskStatusDisabled:
		return "disabled"
	case TaskStatusRunning:
		return "running"
	case TaskStatusCompleted:
		return "completed"
	default:
		return "unspecified"
	}
}

// TaskPriority is an alias for [core/scheduler.TaskPriority]. It controls the
// order in which tasks are dispatched when concurrency slots are contended.
// Higher numeric values represent higher priority.
type TaskPriority = corescheduler.TaskPriority

const (
	// TaskPriorityUnspecified is the zero value; tasks registered without an
	// explicit priority default to [TaskPriorityNormal].
	TaskPriorityUnspecified = corescheduler.TaskPriorityUnspecified
	// TaskPriorityLow tasks execute only when no Normal or High tasks are
	// waiting, and are deferred (not blocked) when all shared slots are full.
	TaskPriorityLow = corescheduler.TaskPriorityLow
	// TaskPriorityNormal is the default priority for tasks that do not specify
	// an explicit level.
	TaskPriorityNormal = corescheduler.TaskPriorityNormal
	// TaskPriorityHigh tasks may use reserved concurrency slots in addition to
	// the shared pool, and are dispatched before Normal and Low tasks.
	TaskPriorityHigh = corescheduler.TaskPriorityHigh
	// TaskPriorityCritical tasks bypass all concurrency limits and execute
	// immediately regardless of slot availability.
	TaskPriorityCritical = corescheduler.TaskPriorityCritical
)

// TaskState represents the persistent state of a scheduled task as stored by a
// [Storage] backend. It embeds [TaskSummary] for the shared summary fields and
// adds detail fields (LastRunID, Meta, timestamps) that are only relevant when
// inspecting a single task. Fields with JSON tags are serialized by storage
// implementations that use JSON encoding.
type TaskState struct {
	TaskSummary
	LastRunID    string            `json:"last_run_id,omitempty"`
	RunStartedAt int64             `json:"run_started_at,omitempty"`
	Meta         map[string]string `json:"meta,omitempty"`
	CreatedAt    int64             `json:"created_at"`
	UpdatedAt    int64             `json:"updated_at"`
}

// TaskHistory represents a record of a single task execution, including timing
// information and whether the execution succeeded. History entries are created
// automatically by the scheduler unless the task has DisableHistory set.
// They are pruned periodically according to the retention configured via
// [WithHistoryRetention].
type TaskHistory struct {
	ID         string `json:"id"` // Unique ID for this history entry
	TaskID     string `json:"task_id"`
	RunID      string `json:"run_id"`
	Error      string `json:"error,omitempty"`
	StartedAt  int64  `json:"started_at"`
	EndedAt    int64  `json:"ended_at"`
	DurationMs int64  `json:"duration_ms"`
	Success    bool   `json:"success"`
}

// TaskSummary provides a read-only summary view of a task, suitable for listing
// endpoints and management UIs. It mirrors a subset of [TaskState] fields but
// omits mutable execution bookkeeping (e.g., LastRunID, Meta) to keep listing
// payloads lightweight.
type TaskSummary struct {
	ID             string       `json:"id"`
	Description    string       `json:"description,omitempty"`
	Status         TaskStatus   `json:"status"`
	Priority       TaskPriority `json:"priority"`
	Schedule       string       `json:"schedule"`
	LastRunAt      int64        `json:"last_run_at,omitempty"`
	NextRunAt      int64        `json:"next_run_at,omitempty"`
	SkipNextRun    bool         `json:"skip_next_run,omitempty"`
	DisableHistory bool         `json:"disable_history,omitempty"`
	Unmanaged      bool         `json:"unmanaged,omitempty"`
	OneShot        bool         `json:"one_shot,omitempty"`
	Failures       int32        `json:"failures"`
}

// TaskFilterFields lists the CEL field names that storage backends must
// accept in task list filter expressions. The names use proto-style
// camelCase and correspond to the fields of [TaskSummary].
var TaskFilterFields = []string{
	"id", "description", "status", "priority", "schedule",
	"lastRunAt", "nextRunAt", "skipNextRun",
	"disableHistory", "unmanaged", "oneShot", "failures",
}

// HistoryFilterFields lists the CEL field names that storage backends
// must accept in history list filter expressions. The names use
// proto-style camelCase and correspond to the fields of [TaskHistory].
var HistoryFilterFields = []string{
	"id", "taskId", "runId", "startedAt", "endedAt",
	"durationMs", "success", "error",
}

// Storage defines the persistence interface for task state and execution history.
// Implementations must be safe for concurrent use by multiple goroutines, as the
// scheduler reads and writes state from the main loop, task goroutines, and
// management methods concurrently.
//
// Iterator methods return [iter.Seq2] for memory-efficient streaming. Callers
// may use slices.Collect to materialize results when a full snapshot is needed.
type Storage interface {
	// GetTask retrieves the state of a specific task by ID. It returns
	// (nil, nil) when the task does not exist.
	GetTask(ctx context.Context, id string) (*TaskState, error)

	// UpsertTask creates or replaces the state of a task. The implementation
	// must treat [TaskState.ID] as the primary key.
	UpsertTask(ctx context.Context, state *TaskState) error

	// DeleteTask removes a task and its associated state from storage.
	// Deleting a non-existent task should be a no-op (no error).
	DeleteTask(ctx context.Context, id string) error

	// Tasks returns an iterator over all stored task states. The iteration
	// order is implementation-defined. Errors encountered mid-iteration are
	// yielded as the second element.
	Tasks(ctx context.Context) iter.Seq2[*TaskState, error]

	// AddHistory records a completed task execution as a [TaskHistory] entry.
	AddHistory(ctx context.Context, history *TaskHistory) error

	// History returns an iterator over execution history entries for the given
	// task ID, ordered by start time descending (most recent first). Errors
	// encountered mid-iteration are yielded as the second element.
	History(ctx context.Context, id string) iter.Seq2[*TaskHistory, error]

	// CleanupHistory removes history entries whose EndedAt timestamp is older
	// than the given retention duration relative to the current time.
	CleanupHistory(ctx context.Context, retention time.Duration) error

	// TasksPaginated returns up to (pg.Limit+1) task states whose ID is
	// lexicographically greater than pg.AfterID, sorted by ID ascending.
	// When f is non-nil the storage should apply the filter predicate at
	// the query level (e.g. bson.M for MongoDB, RediSearch query for Redis,
	// in-memory evaluator for the memory backend). Callers use the extra
	// item to determine whether a next page exists.
	TasksPaginated(ctx context.Context, pg Pagination, f Node) ([]*TaskState, error)

	// HistoryPaginated returns up to (pg.Limit+1) history entries for taskID,
	// sorted by StartedAt descending with ID descending as a tie-breaker,
	// starting after the cursor position in pg. When f is non-nil the storage
	// should apply the filter predicate at the query level.
	HistoryPaginated(ctx context.Context, taskID string, pg HistoryPagination, f Node) ([]*TaskHistory, error)
}

// generateID generates a cryptographically secure random ID.
// Returns a 32-character hex string (128 bits of entropy).
// Panics if the system CSPRNG is unavailable — this mirrors the behavior of
// crypto/rand in Go 1.24+ where Read never returns an error under normal
// operating conditions.
// idBufPool reuses 16-byte buffers for hex ID generation, avoiding a small
// heap allocation on every task execution.
var idBufPool = sync.Pool{New: func() any { return new([16]byte) }}

func generateID() string {
	bp := idBufPool.Get().(*[16]byte) //nolint:errcheck // type guaranteed by pool
	_, _ = rand.Read(bp[:])
	s := hex.EncodeToString(bp[:])
	idBufPool.Put(bp)
	return s
}
