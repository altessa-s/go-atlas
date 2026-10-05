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
	LastRunID    string `json:"last_run_id,omitempty"`
	RunStartedAt int64  `json:"run_started_at,omitempty"`
	// RunLeaseUntil is the Unix second until which the instance executing the
	// unfinished run (RunStartedAt != 0) vouches for it. [Storage.ClaimRun]
	// sets it with the claim and the owner renews it through [Storage.RenewRun]
	// while the run is in flight; stale recovery on any instance leaves the run
	// alone until it has passed. It is meaningful only while RunStartedAt != 0
	// (the bundled [Storage.FinishRun] implementations currently leave the last
	// value in place). Zero on an unfinished run means it was claimed by a release without leases, for which recovery
	// falls back to RunStartedAt plus its own stale timeout.
	RunLeaseUntil int64 `json:"run_lease_until,omitempty"`
	// RunAt is the occurrence a one-shot task was registered for, as Unix
	// seconds before any clamping to the registration time; always positive for
	// one-shot tasks registered by this release and zero for periodic tasks.
	// [Scheduler.Register] compares it to decide whether re-registering a
	// completed one-shot task schedules a new run.
	RunAt     int64             `json:"run_at,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
	CreatedAt int64             `json:"created_at"`
	UpdatedAt int64             `json:"updated_at"`
	// Revision is incremented atomically by the storage on every write
	// (UpsertTask, CreateTask, ClaimRun, RenewRun, FinishRun, ReplaceTaskIf).
	// Values set by callers are ignored; it lets ReplaceTaskIf detect any
	// concurrent change.
	Revision int64 `json:"revision,omitempty"`
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
//
// Run ownership. ClaimRun, RenewRun and FinishRun share the rules below; every
// backend must evaluate each rule and its write as one atomic operation, and
// a GetTask check followed by UpsertTask never satisfies them.
//
//   - Claimable: Status == [TaskStatusActive], RunStartedAt == 0 (no earlier run
//     is unfinished, even one whose task was paused and resumed meanwhile)
//     and, when [RunClaim.NextRunAt] is non-zero, the stored NextRunAt and
//     RunAt equal the claim's (the occurrence fence). Absent fields are zero.
//   - Owned: runID is non-empty, LastRunID == runID and RunStartedAt != 0.
//     The scheduler only stores a positive Unix second in RunStartedAt; the
//     MongoDB backend matches run_started_at > 0, which agrees for every value
//     the scheduler writes.
//   - Finish transition: a one-shot task whose stored RunAt equals
//     [RunResult.RunAt] has executed its occurrence and becomes
//     [TaskStatusCompleted] with NextRunAt zero, whatever its status.
//     Otherwise a [TaskStatusRunning] task returns to [TaskStatusActive] and
//     any status set by management meanwhile is kept. A periodic task whose
//     Schedule still equals [RunResult.Schedule] takes [RunResult.NextRunAt];
//     any other task keeps its NextRunAt. RunStartedAt becomes zero, LastRunAt
//     and UpdatedAt take [RunResult.StartedAt] and [RunResult.EndedAt],
//     Failures resets on success and increments on failure, and configuration
//     and metadata are preserved. RunLeaseUntil carries no meaning once
//     RunStartedAt is zero; the bundled backends currently leave it in place.
//
// Every write, including a renewal, increments [TaskState.Revision].
type Storage interface {
	// GetTask retrieves the state of a specific task by ID. It returns
	// (nil, nil) when the task does not exist.
	GetTask(ctx context.Context, id string) (*TaskState, error)

	// UpsertTask creates or replaces the state of a task. The implementation
	// must treat [TaskState.ID] as the primary key and atomically set the
	// stored [TaskState.Revision] to the previous revision plus one (one for a
	// new task), ignoring the caller's value. Every other write increments the
	// revision in the same atomic operation as well.
	UpsertTask(ctx context.Context, state *TaskState) error

	// CreateTask atomically inserts state only when no task with
	// [TaskState.ID] exists, storing a revision of one regardless of the
	// caller's value. It returns false, leaving the stored task untouched, when
	// the task already exists. [Scheduler.Register] relies on it so that two
	// instances registering the same new task cannot overwrite a state the other
	// has already claimed; a GetTask check followed by UpsertTask does not
	// satisfy this contract.
	CreateTask(ctx context.Context, state *TaskState) (bool, error)

	// ClaimRun atomically transitions task id from active→running while it is
	// claimable (see Run ownership above), stamping Status = running,
	// RunStartedAt = claim.StartedAt, LastRunID = claim.RunID,
	// RunLeaseUntil = claim.LeaseUntil and UpdatedAt = claim.StartedAt. It
	// returns true iff THIS caller won the claim.
	//
	// The write MUST be a single atomic conditional update (CAS). This makes
	// duplicate execution impossible even when two schedulers dispatch the same
	// occurrence concurrently — for example during a leader-election
	// split-brain window — so leadership becomes a throughput optimization, not
	// a correctness dependency. Implementations return (false, nil) when no
	// document matched (already claimed, advanced, or no longer active).
	//
	// Persisting runID as [TaskState.LastRunID] is part of the contract, not a
	// convenience: once the task body returns, the scheduler re-reads the state
	// and writes its result only while LastRunID still names its own run. The
	// atomic FinishRun check is what stops a run whose task was reclaimed mid-flight (by stale
	// recovery plus a competing claim) from marking the live run finished.
	ClaimRun(ctx context.Context, id string, claim RunClaim) (bool, error)

	// FinishRun atomically applies the finish transition (see Run ownership
	// above) only while runID owns the task's unfinished run. It returns false,
	// writing nothing, for missing, reclaimed or finished runs. A one-shot task
	// completes whatever its status because its single execution has happened —
	// a pause and resume during the run must not leave it due again.
	FinishRun(ctx context.Context, id, runID string, result RunResult) (bool, error)

	// RenewRun atomically sets [TaskState.RunLeaseUntil] to leaseUntil and
	// increments the revision only while runID owns the task's unfinished run
	// (see Run ownership above). It returns false, writing nothing, otherwise. The scheduler calls it
	// periodically for every run it executes so that stale recovery on other
	// instances can tell a long run from an abandoned one.
	RenewRun(ctx context.Context, id, runID string, leaseUntil int64) (bool, error)

	// ReplaceTaskIf atomically replaces the state of task state.ID only while
	// the stored document still matches expect, storing expect.Revision+1 as the
	// new revision. It returns false when the task is missing or any fenced
	// field, including the revision, changed since the caller read it. Missing
	// fields compare equal to their zero values. A GetTask check followed by
	// UpsertTask does not satisfy this contract.
	ReplaceTaskIf(ctx context.Context, state *TaskState, expect TaskFence) (bool, error)

	// DeleteTask removes a task and its associated state from storage.
	// Deleting a non-existent task should be a no-op (no error).
	DeleteTask(ctx context.Context, id string) error

	// Tasks returns an iterator over all stored task states. The iteration
	// order is implementation-defined. Errors encountered mid-iteration are
	// yielded as the second element.
	Tasks(ctx context.Context) iter.Seq2[*TaskState, error]

	// DueTasks returns an iterator over the task states eligible for dispatch
	// at the instant now (a Unix timestamp in seconds): exactly those with
	// Status == [TaskStatusActive] and NextRunAt <= now, ordered by ID
	// ascending. Errors encountered mid-iteration are yielded as the second
	// element.
	//
	// The scheduler calls this on every tick, so the predicate MUST be pushed
	// down to the backend rather than evaluated by filtering the output of
	// Tasks. A tick that fetches the whole collection and discards most of it
	// scales with the number of tasks ever registered instead of the number
	// actually due, once per tick interval, forever.
	//
	// Returning too much is not a correctness hazard on the status axis:
	// ClaimRun's compare-and-swap re-checks status == active, so a paused task
	// that slips through is rejected at claim time. The NextRunAt bound has no
	// such backstop — an implementation that ignores it makes tasks fire early.
	DueTasks(ctx context.Context, now int64) iter.Seq2[*TaskState, error]

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

// TaskFence is the compare-and-swap precondition of Storage.ReplaceTaskIf: the
// run-ownership fields and revision a writer observed when it read the task.
type TaskFence struct {
	Status       TaskStatus
	NextRunAt    int64
	LastRunID    string
	RunStartedAt int64
	Revision     int64
}

// FenceOf returns the [TaskFence] describing state as currently read.
func FenceOf(state *TaskState) TaskFence {
	return TaskFence{
		Status:       state.Status,
		NextRunAt:    state.NextRunAt,
		LastRunID:    state.LastRunID,
		RunStartedAt: state.RunStartedAt,
		Revision:     state.Revision,
	}
}

// RunClaim describes the run [Storage.ClaimRun] starts and the occurrence it
// fences on; see the Run ownership rules of [Storage].
type RunClaim struct {
	// NextRunAt and RunAt identify the occurrence the caller observed as due.
	// The claim succeeds only while both are still stored; a zero NextRunAt
	// disables the occurrence fence.
	NextRunAt int64
	RunAt     int64
	// StartedAt, RunID and LeaseUntil are stored as RunStartedAt, LastRunID
	// and RunLeaseUntil, so the run owns a lease from the moment it exists.
	StartedAt  int64
	RunID      string
	LeaseUntil int64
}

// RunResult contains the execution fields committed by [Storage.FinishRun]
// under the finish transition of [Storage]'s Run ownership rules. Schedule
// identifies the schedule used to compute NextRunAt, so a concurrently changed
// schedule and its next occurrence are preserved.
//
// RunAt identifies the one-shot occurrence the run executed (the claimed
// state's [TaskState.RunAt]; zero for periodic runs). A task re-registered for
// a different occurrence while the run executed keeps that new occurrence
// (status back to active if still running, NextRunAt kept).
type RunResult struct {
	StartedAt int64
	EndedAt   int64
	NextRunAt int64
	RunAt     int64
	Schedule  string
	Success   bool
}
