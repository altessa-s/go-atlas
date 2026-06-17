// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
	natsstore "github.com/altessa-s/go-atlas/data/saga/storages/nats"
)

var errTest = errors.New("boom")

// newStore starts an isolated embedded NATS server with JetStream and returns
// a Store over a fresh bucket. Server lifecycle is bound to the test.
func newStore(t *testing.T) *natsstore.Store {
	t.Helper()
	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)
	s, err := natsstore.New(js, natsstore.WithBucket("saga_test"))
	require.NoError(t, err)
	return s
}

func TestStoreRoundTrip(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()

	inst := &saga.Instance{ID: "o1", Definition: "d", Status: saga.StatusRunning, Data: []byte(`{"n":1}`)}
	require.NoError(t, s.Create(ctx, inst))
	require.Equal(t, int64(1), inst.Version) // first revision

	got, err := s.Get(ctx, "o1")
	require.NoError(t, err)
	require.Equal(t, saga.StatusRunning, got.Status)
	require.Equal(t, int64(1), got.Version)

	got.Status = saga.StatusCompleted
	require.NoError(t, s.Update(ctx, got))
	require.Equal(t, int64(2), got.Version) // revision bumped

	require.NoError(t, s.Delete(ctx, "o1"))
	_, err = s.Get(ctx, "o1")
	require.ErrorIs(t, err, sagaerrs.ErrInstanceNotFound)
	require.NoError(t, s.Delete(ctx, "o1")) // idempotent
}

func TestCreateDuplicate(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()
	require.NoError(t, s.Create(ctx, &saga.Instance{ID: "o1", Status: saga.StatusRunning}))
	require.ErrorIs(t, s.Create(ctx, &saga.Instance{ID: "o1", Status: saga.StatusRunning}), sagaerrs.ErrInstanceExists)
}

func TestVersionConflict(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()
	require.NoError(t, s.Create(ctx, &saga.Instance{ID: "o1", Status: saga.StatusRunning}))

	a, err := s.Get(ctx, "o1")
	require.NoError(t, err)
	b := a.Clone()

	require.NoError(t, s.Update(ctx, a)) // a wins, revision bumped
	err = s.Update(ctx, b)               // b carries the stale revision
	require.ErrorIs(t, err, sagaerrs.ErrVersionConflict)
}

func TestUpdateNotFound(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	require.ErrorIs(t, s.Update(t.Context(), &saga.Instance{ID: "missing", Version: 1}), sagaerrs.ErrInstanceNotFound)
}

func TestFetchRecoverable(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()
	now := time.Now().UTC()

	require.NoError(t, s.Create(ctx, &saga.Instance{ID: "running_ok", Status: saga.StatusRunning}))
	require.NoError(t, s.Create(ctx, &saga.Instance{ID: "running_timed_out", Status: saga.StatusRunning, Deadline: now.Add(-time.Minute)}))
	require.NoError(t, s.Create(ctx, &saga.Instance{ID: "compensating", Status: saga.StatusCompensating}))
	require.NoError(t, s.Create(ctx, &saga.Instance{ID: "completed", Status: saga.StatusCompleted}))

	got, err := s.FetchRecoverable(ctx, now, 0)
	require.NoError(t, err)

	ids := map[string]bool{}
	for _, inst := range got {
		ids[inst.ID] = true
	}
	require.True(t, ids["running_timed_out"])
	require.True(t, ids["compensating"])
	require.False(t, ids["running_ok"])
	require.False(t, ids["completed"])
}

// pay is the shared data type for the end-to-end orchestrator tests.
type pay struct {
	N int `json:"n"`
}

func TestOrchestratorOverNATS(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	bump := func(_ context.Context, d *pay) error { d.N++; return nil }
	noop := func(_ context.Context, _ *pay) error { return nil }

	t.Run("completes and persists", func(t *testing.T) {
		t.Parallel()
		s := newStore(t)
		def := saga.NewDefinition[pay]("place").
			Step("a", bump).Compensate(noop).
			Step("b", bump).
			MustBuild()
		orch := saga.New(s, def)

		inst, err := orch.Start(ctx, "ok1", pay{})
		require.NoError(t, err)
		require.Equal(t, saga.StatusCompleted, inst.Status)

		stored, err := s.Get(ctx, "ok1")
		require.NoError(t, err)
		require.Equal(t, saga.StatusCompleted, stored.Status)
	})

	t.Run("rolls back on failure", func(t *testing.T) {
		t.Parallel()
		s := newStore(t)
		def := saga.NewDefinition[pay]("place").
			Step("a", bump).Compensate(noop).
			Step("b", func(_ context.Context, _ *pay) error { return errTest }).
			MustBuild()
		orch := saga.New(s, def,
			saga.WithMaxStepAttempts(1),
			saga.WithStepRetryBaseDelay(time.Millisecond),
		)

		inst, err := orch.Start(ctx, "rb1", pay{})
		require.ErrorIs(t, err, errTest)
		require.Equal(t, saga.StatusCompensated, inst.Status)

		stored, err := s.Get(ctx, "rb1")
		require.NoError(t, err)
		require.Equal(t, saga.StatusCompensated, stored.Status)
	})
}
