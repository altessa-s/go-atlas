// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/scheduler"
	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/data/saga/storages/memory"

	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
)

func TestParallelFailureCompensatesSuccessfulSibling(t *testing.T) {
	t.Parallel()
	var applied, undone atomic.Bool
	done := make(chan struct{})
	boom := errors.New("step failed")
	def := saga.NewDefinition[struct{}]("partial").Parallel("group",
		saga.NewStep("success", func(context.Context, *struct{}) error {
			applied.Store(true)
			close(done)
			return nil
		}).Compensate(func(context.Context, *struct{}) error { undone.Store(true); return nil }),
		saga.NewStep("failure", func(ctx context.Context, _ *struct{}) error {
			select {
			case <-done:
				return boom
			case <-ctx.Done():
				return ctx.Err()
			}
		}).ReadOnly(),
	).MustBuild()
	o := saga.New(memory.New(), def, saga.WithMaxStepAttempts(1), saga.WithStepConcurrency(2))
	inst, err := o.Start(t.Context(), "one", struct{}{})
	require.ErrorIs(t, err, boom)
	require.Equal(t, saga.StatusCompensated, inst.Status)
	require.True(t, applied.Load())
	require.True(t, undone.Load())
}

func TestRecoveryCannotOvertakeLiveAction(t *testing.T) {
	t.Parallel()
	entered, release := make(chan struct{}), make(chan struct{})
	var applied atomic.Bool
	def := saga.NewDefinition[struct{}]("overlap").Step("reserve", func(ctx context.Context, _ *struct{}) error {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		applied.Store(true)
		return nil
	}).Compensate(func(context.Context, *struct{}) error { applied.Store(false); return nil }).MustBuild()
	store := memory.New()
	o := saga.New(store, def, saga.WithSagaTimeout(time.Nanosecond), saga.WithMaxStepAttempts(1))
	finished := make(chan error, 1)
	go func() { _, err := o.Start(t.Context(), "one", struct{}{}); finished <- err }()
	<-entered
	other := saga.New(store, def)
	require.NoError(t, other.RunRecoveryCycle(t.Context()))
	_, err := other.Resume(t.Context(), "one")
	require.ErrorIs(t, err, sagaerrs.ErrInstanceBusy)
	close(release)
	require.NoError(t, <-finished)
	inst, err := store.Get(t.Context(), "one")
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompleted, inst.Status)
	require.True(t, applied.Load())
}

type rejectedRegistrar struct{}

func (rejectedRegistrar) Register(context.Context, scheduler.TaskConfig) error {
	return errors.New("registration rejected")
}

func TestRegistrationFailureKeepsManualRecoveryAvailable(t *testing.T) {
	t.Parallel()
	def := saga.NewDefinition[struct{}]("registration").Step("noop", func(context.Context, *struct{}) error { return nil }).ReadOnly().MustBuild()
	o := saga.New(memory.New(), def, saga.WithScheduler(rejectedRegistrar{}), saga.WithRecoverySchedule("@every 1m"))
	require.Error(t, o.RegisterRecovery(t.Context()))
	require.NoError(t, o.RunRecoveryCycle(t.Context()))
}

func TestInterruptedExecutionWithoutDeadlineRemainsRecoverable(t *testing.T) {
	t.Parallel()
	store := memory.New()
	var calls atomic.Int32
	var first, second saga.Execution
	def := saga.NewDefinition[struct{}]("interrupted").Step("step", func(ctx context.Context, _ *struct{}) error {
		e, _ := saga.ExecutionFromContext(ctx)
		if calls.Add(1) == 1 {
			first = e
			<-ctx.Done()
			return ctx.Err()
		}
		second = e
		return nil
	}).ReadOnly().MustBuild()
	o := saga.New(store, def, saga.WithExecutionTimeout(10*time.Millisecond), saga.WithMaxStepAttempts(1))
	_, err := o.Start(t.Context(), "one", struct{}{})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	inst, err := store.Get(t.Context(), "one")
	require.NoError(t, err)
	require.True(t, inst.Recoverable(time.Now()))
	require.Equal(t, []int{0}, inst.PendingSteps)
	require.NoError(t, saga.New(store, def).RunRecoveryCycle(t.Context()))
	inst, err = store.Get(t.Context(), "one")
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompleted, inst.Status)
	require.Greater(t, second.Fence, first.Fence)
	require.Equal(t, first.StepKey, second.StepKey)
}

func TestRecoveryCompensatesUncheckpointedStageIntent(t *testing.T) {
	t.Parallel()
	store := memory.New()
	var undos atomic.Int32
	def := saga.NewDefinition[struct{}]("intent").Step("reserve", func(context.Context, *struct{}) error { return nil }).Compensate(func(context.Context, *struct{}) error { undos.Add(1); return nil }).MustBuild()
	require.NoError(t, store.Create(t.Context(), &saga.Instance{ID: "one", Definition: "intent", Status: saga.StatusRunning, PendingSteps: []int{0}, LeaseOwner: "dead", LeaseUntil: time.Now().Add(-time.Hour), Deadline: time.Now().Add(-time.Minute)}))
	require.NoError(t, saga.New(store, def).RunRecoveryCycle(t.Context()))
	inst, err := store.Get(t.Context(), "one")
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompensated, inst.Status)
	require.Equal(t, int32(1), undos.Load())
}

type blockingRecoveryStore struct{ saga.Storage }

func (s blockingRecoveryStore) FetchRecoverable(ctx context.Context, _ string, _ time.Time, _ int) ([]*saga.Instance, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func TestRecoveryIOHasBoundedContext(t *testing.T) {
	t.Parallel()
	def := saga.NewDefinition[struct{}]("bound").Step("noop", func(context.Context, *struct{}) error { return nil }).ReadOnly().MustBuild()
	o := saga.New(blockingRecoveryStore{memory.New()}, def, saga.WithRecoveryTimeout(10*time.Millisecond))
	require.ErrorIs(t, o.RunRecoveryCycle(t.Context()), context.DeadlineExceeded)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, o.RunRecoveryCycle(ctx), context.Canceled)
}

// terminalWriteFailStore fails the first Update that would persist the given
// terminal status, simulating an outage on the final checkpoint.
type terminalWriteFailStore struct {
	saga.Storage
	status saga.Status
	failed atomic.Bool
}

func (s *terminalWriteFailStore) Update(ctx context.Context, inst *saga.Instance) error {
	if inst.Status == s.status && s.failed.CompareAndSwap(false, true) {
		return errors.New("terminal write failed")
	}
	return s.Storage.Update(ctx, inst)
}

func TestFailedTerminalWriteLeavesInstanceNonTerminal(t *testing.T) {
	t.Parallel()
	boom := errors.New("step failed")
	for _, tc := range []struct {
		name     string
		terminal saga.Status
		want     saga.Status
		failStep error
	}{
		{"completed", saga.StatusCompleted, saga.StatusRunning, nil},
		{"compensated", saga.StatusCompensated, saga.StatusCompensating, boom},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &terminalWriteFailStore{Storage: memory.New(), status: tc.terminal}
			def := saga.NewDefinition[struct{}]("terminal").
				Step("a", func(context.Context, *struct{}) error { return nil }).
				Compensate(func(context.Context, *struct{}) error { return nil }).
				Step("b", func(context.Context, *struct{}) error { return tc.failStep }).
				Compensate(func(context.Context, *struct{}) error { return nil }).
				MustBuild()
			o := saga.New(store, def, saga.WithMaxStepAttempts(1))

			inst, err := o.Start(t.Context(), "id", struct{}{})
			require.Error(t, err)
			require.Equal(t, tc.want, inst.Status, "an unpersisted terminal status must not be reported")
			stored, err := store.Get(t.Context(), "id")
			require.NoError(t, err)
			require.Equal(t, tc.want, stored.Status)

			inst, _ = o.Resume(t.Context(), "id")
			require.Equal(t, tc.terminal, inst.Status)
		})
	}
}
