// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/data/saga/storages/memory"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// twoStepDef builds a two-step saga (both compensatable) used by the recovery
// tests. Stage 0 = "a", stage 1 = "b".
func twoStepDef(r *recorder) *saga.Definition[order] {
	return saga.NewDefinition[order]("recover").
		Step("a", doStep(r, "a", nil)).Compensate(undoStep(r, "a")).
		Step("b", doStep(r, "b", nil)).Compensate(undoStep(r, "b")).
		MustBuild()
}

// craftInstance stores a half-completed instance (stage 0 done) directly in the
// store, simulating a process that crashed mid-saga.
func craftInstance(t *testing.T, store *memory.Store, status saga.Status, deadline time.Time) {
	t.Helper()
	data, err := json.Marshal(&order{})
	require.NoError(t, err)
	past := time.Now().Add(-time.Hour).UTC()
	require.NoError(t, store.Create(t.Context(), &saga.Instance{
		ID:         "r1",
		Definition: "recover",
		Status:     status,
		Stage:      1, // stage 0 ("a") already committed
		Data:       data,
		CreatedAt:  past,
		UpdatedAt:  past,
		Deadline:   deadline,
	}))
}

func TestRecoveryAutoRollbackOnTimeout(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	store := memory.New()

	// A Running instance whose deadline has passed must be rolled back.
	craftInstance(t, store, saga.StatusRunning, time.Now().Add(-time.Minute).UTC())

	orch := saga.New(store, twoStepDef(r), fastOpts()...)
	require.NoError(t, orch.RunRecoveryCycle(t.Context()))

	inst, err := store.Get(t.Context(), "r1")
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompensated, inst.Status)
	require.Equal(t, []string{"undo:a"}, r.snapshot())
}

func TestRecoveryResumesCompensating(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	store := memory.New()

	// An instance left mid-compensation (no deadline) is finished by recovery.
	craftInstance(t, store, saga.StatusCompensating, time.Time{})

	orch := saga.New(store, twoStepDef(r), fastOpts()...)
	require.NoError(t, orch.RunRecoveryCycle(t.Context()))

	inst, err := store.Get(t.Context(), "r1")
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompensated, inst.Status)
	require.Equal(t, []string{"undo:a"}, r.snapshot())
}

func TestRecoveryIgnoresHealthyRunning(t *testing.T) {
	t.Parallel()
	r := &recorder{}
	store := memory.New()

	// A Running instance with no deadline is not recoverable and is left alone.
	craftInstance(t, store, saga.StatusRunning, time.Time{})

	orch := saga.New(store, twoStepDef(r), fastOpts()...)
	require.NoError(t, orch.RunRecoveryCycle(t.Context()))

	inst, err := store.Get(t.Context(), "r1")
	require.NoError(t, err)
	require.Equal(t, saga.StatusRunning, inst.Status)
	require.Empty(t, r.snapshot())
}

// fakeElector is a [saga.LeaderElector] whose leadership is fixed at construction.
type fakeElector struct{ leader bool }

func (g fakeElector) IsLeader() bool { return g.leader }

func TestRecoveryLeaderElector(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		leader     bool
		wantStatus saga.Status
		wantUndo   []string
	}{
		{name: "non-leader skips the cycle", leader: false, wantStatus: saga.StatusRunning, wantUndo: nil},
		{name: "leader runs the cycle", leader: true, wantStatus: saga.StatusCompensated, wantUndo: []string{"undo:a"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := &recorder{}
			store := memory.New()

			// A Running instance past its deadline is recoverable; the gate
			// decides whether this node acts on it.
			craftInstance(t, store, saga.StatusRunning, time.Now().Add(-time.Minute).UTC())

			opts := append(fastOpts(), saga.WithLeaderElector(fakeElector{leader: tc.leader}))
			orch := saga.New(store, twoStepDef(r), opts...)
			require.NoError(t, orch.RunRecoveryCycle(t.Context()))

			inst, err := store.Get(t.Context(), "r1")
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, inst.Status)
			require.Equal(t, tc.wantUndo, r.snapshot())
		})
	}
}

// fakeRegistrar is a no-op [corescheduler.TaskRegistrar] that records calls.
type fakeRegistrar struct {
	registered int
	lastID     string
}

func (f *fakeRegistrar) Register(_ context.Context, cfg corescheduler.TaskConfig) error {
	f.registered++
	f.lastID = cfg.ID
	return nil
}

func TestRunRecoveryCycleSchedulerManaged(t *testing.T) {
	t.Parallel()
	reg := &fakeRegistrar{}

	orch := saga.New(memory.New(), twoStepDef(&recorder{}),
		saga.WithScheduler(reg),
		saga.WithRecoverySchedule("@every 1m"),
		saga.WithRecoveryTaskID("saga-recovery-test"),
	)

	require.Equal(t, 1, reg.registered)
	require.Equal(t, "saga-recovery-test", reg.lastID)

	err := orch.RunRecoveryCycle(t.Context())
	require.ErrorIs(t, err, saga.ErrSchedulerManaged)
}
