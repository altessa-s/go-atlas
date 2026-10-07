// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strconv"
	"time"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
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

// execute acquires ownership before inspecting deadlines or invoking actions.
// The lease is deliberately not renewed: one invocation is bounded, and a
// subsequent Resume gets a new fence. Grace covers cancellation and clock skew;
// callbacks must stop on cancellation or enforce fencing at their backend.
func (o *Orchestrator[T]) execute(ctx context.Context, inst *Instance, data *T, recovering bool) (result *Instance, err error) {
	if inst.Stage < 0 || inst.Stage > len(o.def.stages) || (len(inst.PendingSteps) > 0 && inst.Stage == len(o.def.stages)) {
		return inst, fmt.Errorf("saga: invalid persisted stage %d", inst.Stage)
	}
	for _, i := range inst.PendingSteps {
		if i < 0 || i >= len(o.def.stages[inst.Stage].steps) {
			return inst, fmt.Errorf("saga: invalid pending step %d", i)
		}
	}
	now := time.Now().UTC()
	if inst.LeaseUntil.After(now) {
		return inst, sagaerrs.ErrInstanceBusy
	}
	runCtx, cancel := corecontext.WithMaxTimeout(ctx, o.executionTimeout)
	defer cancel()
	deadline, _ := runCtx.Deadline()
	inst.LeaseOwner = rand.Text()
	inst.LeaseUntil = deadline.Add(o.leaseGrace)
	if updateErr := o.store.Update(runCtx, inst); updateErr != nil {
		return inst, updateErr
	}
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
func (s boundedStorage) FetchRecoverable(ctx context.Context, now time.Time, limit int) ([]*Instance, error) {
	ctx, cancel := corecontext.WithMaxTimeout(ctx, s.timeout)
	defer cancel()
	return s.Storage.FetchRecoverable(ctx, now, limit)
}
