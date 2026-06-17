// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga

import (
	"context"
	"slices"
	"time"
)

// Status is the lifecycle state of a saga [Instance].
//
// The state machine is:
//
//	Running ──success──▶ Completed            (terminal)
//	Running ──failure──▶ Compensating
//	Compensating ─done─▶ Compensated          (terminal)
//	Compensating ─err──▶ Failed               (terminal, needs intervention)
//	Running ─pivot-exhausted─▶ Failed         (terminal, roll-forward gave up)
type Status string

const (
	// StatusRunning means the saga is executing forward stages.
	StatusRunning Status = "RUNNING"
	// StatusCompensating means a step failed before the pivot and the saga is
	// rolling back completed stages in reverse order.
	StatusCompensating Status = "COMPENSATING"
	// StatusCompleted means every forward stage committed successfully.
	StatusCompleted Status = "COMPLETED"
	// StatusCompensated means every completed stage was successfully rolled back.
	StatusCompensated Status = "COMPENSATED"
	// StatusFailed means the saga is in an unrecoverable state: a compensation
	// exhausted its retries, or a post-pivot stage could not roll forward. It
	// requires manual intervention and triggers the dead-letter hook.
	StatusFailed Status = "FAILED"
)

// IsTerminal reports whether the status is final and no further work will run.
func (s Status) IsTerminal() bool {
	switch s {
	case StatusCompleted, StatusCompensated, StatusFailed:
		return true
	default:
		return false
	}
}

// StepStatus is the per-step outcome recorded on an [Instance].
type StepStatus string

const (
	// StepPending means the step has not completed yet.
	StepPending StepStatus = "PENDING"
	// StepCompleted means the step's action committed successfully.
	StepCompleted StepStatus = "COMPLETED"
	// StepCompensated means the step's compensation ran successfully.
	StepCompensated StepStatus = "COMPENSATED"
	// StepFailed means the step's action or compensation gave up after retries.
	StepFailed StepStatus = "FAILED"
)

// StepRecord captures the execution history of a single step within an
// [Instance]. Records are appended in execution order; the orchestrator uses
// them only for observability and debugging, not for control flow (the
// authoritative cursor is [Instance.Stage]).
type StepRecord struct {
	// Name is the step name from the definition.
	Name string `json:"name"`
	// Stage is the zero-based stage index the step belongs to.
	Stage int `json:"stage"`
	// Status is the latest outcome for the step.
	Status StepStatus `json:"status"`
	// Attempts counts how many times the action (or compensation) was invoked.
	Attempts int `json:"attempts"`
	// Error holds the last error string, empty on success.
	Error string `json:"error,omitempty"`
	// StartedAt and FinishedAt bound the most recent invocation (UTC).
	StartedAt  time.Time `json:"started_at,omitzero"`
	FinishedAt time.Time `json:"finished_at,omitzero"`
}

// Instance is the persisted state of a single saga execution. It is the unit
// of storage: a [Store] reads and writes whole instances. Backends treat
// [Instance.Data] as an opaque blob — the orchestrator owns serialization of
// the typed saga payload into and out of it.
type Instance struct {
	// ID uniquely identifies the saga execution. Callers supply it to
	// Orchestrator.Start and it doubles as the idempotency key.
	ID string `json:"id"`
	// Definition is the name of the saga definition this instance runs. Resume
	// rejects an instance whose Definition does not match the orchestrator's.
	Definition string `json:"definition"`
	// Status is the current lifecycle state.
	Status Status `json:"status"`
	// Stage is the cursor. While Running it is the index of the next forward
	// stage to execute (equivalently, the count of committed stages). While
	// Compensating it is the exclusive upper bound of stages still to be
	// compensated (compensation walks Stage-1 down to 0).
	Stage int `json:"stage"`
	// Data is the serialized saga payload (type T). Backends never interpret it.
	Data []byte `json:"data,omitempty"`
	// Steps is the append-only execution history (observability only).
	Steps []StepRecord `json:"steps,omitempty"`
	// CreatedAt and UpdatedAt are maintained by the orchestrator (UTC).
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// Deadline, when non-zero, is the wall-clock time after which a still-Running
	// instance is eligible for automatic rollback by the recovery loop.
	Deadline time.Time `json:"deadline,omitzero"`
	// Version is the optimistic-concurrency token. Store.Update must reject a
	// write whose Version no longer matches the persisted value and increment
	// it on success.
	Version int64 `json:"version"`
	// LastError holds the most recent error string for diagnostics.
	LastError string `json:"last_error,omitempty"`
}

// Clone returns a deep copy of the instance. Storage backends use it to avoid
// handing out references to their internal state (and callers mutating it).
func (i *Instance) Clone() *Instance {
	if i == nil {
		return nil
	}
	out := *i
	out.Data = slices.Clone(i.Data)
	out.Steps = slices.Clone(i.Steps)
	return &out
}

// Store is the persistence backend for saga instances. Implementations live in
// data/saga/storages/* and must be safe for concurrent use by multiple
// goroutines. The interface is defined here, on the consumer side, so backends
// depend on saga and not the reverse.
//
// All methods return the sentinel errors from data/saga/errs where documented;
// callers match them with [errors.Is].
type Store interface {
	// Create persists a brand-new instance. It returns errs.ErrInstanceExists
	// when an instance with the same ID is already stored, which lets Start be
	// idempotent on the saga ID.
	Create(ctx context.Context, inst *Instance) error

	// Get loads an instance by ID, returning errs.ErrInstanceNotFound when
	// absent. The returned instance is owned by the caller (a copy).
	Get(ctx context.Context, id string) (*Instance, error)

	// Update persists inst using optimistic concurrency: it must compare the
	// supplied Version against the stored value, return errs.ErrVersionConflict
	// on mismatch, and otherwise overwrite the record and bump the stored
	// Version. The supplied inst.Version is the version the caller last read;
	// implementations set inst.Version to the new value on success.
	Update(ctx context.Context, inst *Instance) error

	// FetchRecoverable returns up to limit non-terminal instances that are
	// candidates for recovery: those whose Deadline is non-zero and at or
	// before now, plus any left mid-compensation. The recovery loop resumes or
	// rolls them back. Order is unspecified.
	//
	// Durable backends (mongo, redis) persist the deadline as Unix seconds and
	// so compare it at one-second granularity; the in-memory backend uses full
	// time precision. This only affects sub-second deadlines, which the
	// minute-scale recovery cadence makes immaterial in practice.
	FetchRecoverable(ctx context.Context, now time.Time, limit int) ([]*Instance, error)

	// Delete removes a (typically terminal) instance for retention. Deleting a
	// missing instance is not an error.
	Delete(ctx context.Context, id string) error
}
