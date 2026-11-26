// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	corecontext "github.com/altessa-s/go-atlas-core/context"
)

// Execution describes the current action or compensation. Fence increases
// across ownership acquisitions of an instance. External systems supporting
// fencing should reject older tokens; StepKey is stable across retries and
// resumes and can be used as an idempotency key within each operation type.
type Execution struct {
	InstanceID string
	Fence      int64
	StepKey    string
	stage      int
}

type executionKey struct{}

// ExecutionFromContext returns execution metadata inside a saga callback.
func ExecutionFromContext(ctx context.Context) (Execution, bool) {
	e, ok := ctx.Value(executionKey{}).(Execution)
	return e, ok
}

func withStage(ctx context.Context, stage int) context.Context {
	e, _ := ExecutionFromContext(ctx)
	e.stage = stage
	return context.WithValue(ctx, executionKey{}, e)
}

func withStep(ctx context.Context, name string) context.Context {
	e, _ := ExecutionFromContext(ctx)
	e.StepKey = strconv.Itoa(len(e.InstanceID)) + ":" + e.InstanceID + ":" + strconv.Itoa(e.stage) + ":" + name
	return context.WithValue(ctx, executionKey{}, e)
}

var (
	errInvalidStage       = errors.New("saga: invalid persisted stage")
	errInvalidPendingStep = errors.New("saga: invalid pending step")
)

// leaseAcquireError marks a failure of the first lease write of an execution,
// before any step ran.
type leaseAcquireError struct{ err error }

func (e *leaseAcquireError) Error() string { return e.err.Error() }
func (e *leaseAcquireError) Unwrap() error { return e.err }

// validate checks that the persisted cursor addresses stages and steps of the
// bound definition.
func (o *Orchestrator[T]) validate(inst *Instance) error {
	if inst.Stage < 0 || inst.Stage > len(o.def.stages) || (len(inst.PendingSteps) > 0 && inst.Stage == len(o.def.stages)) {
		return fmt.Errorf("%w %d", errInvalidStage, inst.Stage)
	}
	for _, i := range inst.PendingSteps {
		if i < 0 || i >= len(o.def.stages[inst.Stage].steps) {
			return fmt.Errorf("%w %d", errInvalidPendingStep, i)
		}
	}
	return nil
}

// execute acquires ownership of a persisted instance before inspecting
// deadlines or invoking actions: a held lease is busy, otherwise the lease is
// taken with a version-checked write. A persisted lease owner never authorizes
// skipping this; only [Orchestrator.Start] owns the lease it just created.
func (o *Orchestrator[T]) execute(ctx context.Context, inst *Instance, data *T, recovering bool) (*Instance, error) {
	if err := o.validate(inst); err != nil {
		return inst, err
	}
	now := time.Now().UTC()
	if inst.LeaseUntil.After(now) {
		return inst, ErrInstanceBusy
	}
	runCtx, cancel := corecontext.WithMaxTimeout(ctx, o.executionTimeout)
	defer cancel()
	inst.LeaseOwner, inst.LeaseUntil = o.newLease(runCtx)
	if err := o.store.Update(runCtx, inst); err != nil {
		return inst, &leaseAcquireError{err: err}
	}
	return o.executeOwned(ctx, runCtx, inst, data, recovering, now)
}

// newLease returns a fresh lease owner and its expiry for an execution bounded
// by runCtx. The lease is deliberately not renewed: one invocation is bounded,
// and a subsequent Resume gets a new fence. Grace covers cancellation and clock
// skew; callbacks must stop on cancellation or enforce fencing at their backend.
func (o *Orchestrator[T]) newLease(runCtx context.Context) (owner string, until time.Time) {
	deadline, _ := runCtx.Deadline()
	return rand.Text(), deadline.Add(o.leaseGrace)
}

// executeOwned runs an instance whose lease (inst.LeaseOwner) this caller holds
// durably and releases it afterwards. ctx is the caller's context, runCtx the
// execution-bounded one.
func (o *Orchestrator[T]) executeOwned(ctx, runCtx context.Context, inst *Instance, data *T, recovering bool, now time.Time) (result *Instance, err error) {
	owner := inst.LeaseOwner
	defer func() {
		// Reload rather than committing in-memory changes from a failed checkpoint.
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), o.storageTimeout)
		defer cancel()
		current, releaseErr := o.store.Get(releaseCtx, inst.ID)
		if releaseErr == nil && current.LeaseOwner == owner {
			// Interrupted executions stay discoverable even without a saga deadline.
			current.LeaseUntil = time.Now().UTC()
			if current.Status.IsTerminal() {
				current.LeaseOwner = ""
				current.LeaseUntil = time.Time{}
			}
			releaseErr = o.store.Update(releaseCtx, current)
			if releaseErr == nil {
				inst.LeaseOwner = current.LeaseOwner
				inst.LeaseUntil = current.LeaseUntil
				inst.Version = current.Version
			}
		}
		if releaseErr != nil && inst.Status.IsTerminal() {
			// The terminal checkpoint is durable and the lease only lingers until
			// it expires, so the outcome stands.
			o.logger.WarnContext(ctx, "saga: lease release failed after a terminal checkpoint",
				slog.String("id", inst.ID), slog.Any("error", releaseErr))
			releaseErr = nil
		}
		err = errors.Join(err, releaseErr)
	}()
	runCtx = context.WithValue(runCtx, executionKey{}, Execution{InstanceID: inst.ID, Fence: inst.Version})
	if recovering && inst.Status == StatusRunning && !inst.Deadline.IsZero() && !now.Before(inst.Deadline) && inst.Stage < o.def.pivotIdx {
		inst.Status = StatusCompensating
		inst.LastError = "saga: deadline exceeded; rolling back"
		if err := o.store.Update(runCtx, inst); err != nil {
			return inst, err
		}
	}
	return o.drive(runCtx, inst, data)
}

// boundedStorage applies the persistence budget at every I/O boundary, including
// recovery scans and lease cleanup, while preserving earlier caller deadlines.
type boundedStorage struct {
	Storage
	timeout time.Duration
}

func (s boundedStorage) Create(ctx context.Context, inst *Instance) error {
	ctx, cancel := corecontext.WithMaxTimeout(ctx, s.timeout)
	defer cancel()
	return s.Storage.Create(ctx, inst)
}
func (s boundedStorage) Get(ctx context.Context, id string) (*Instance, error) {
	ctx, cancel := corecontext.WithMaxTimeout(ctx, s.timeout)
	defer cancel()
	return s.Storage.Get(ctx, id)
}
func (s boundedStorage) Update(ctx context.Context, inst *Instance) error {
	ctx, cancel := corecontext.WithMaxTimeout(ctx, s.timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.Storage.Update(ctx, inst)
}
func (s boundedStorage) DeleteTerminal(ctx context.Context, definition string, olderThan time.Time, limit int) (int, error) {
	ctx, cancel := corecontext.WithMaxTimeout(ctx, s.timeout)
	defer cancel()
	return s.Storage.DeleteTerminal(ctx, definition, olderThan, limit)
}
func (s boundedStorage) FetchRecoverable(ctx context.Context, definition string, now time.Time, limit int) ([]*Instance, error) {
	ctx, cancel := corecontext.WithMaxTimeout(ctx, s.timeout)
	defer cancel()
	return s.Storage.FetchRecoverable(ctx, definition, now, limit)
}
