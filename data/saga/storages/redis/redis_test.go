// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
	sagaredis "github.com/altessa-s/go-atlas/data/saga/storages/redis"
)

var baseTime = time.Unix(1_700_000_000, 0).UTC()

func newStore(tb testing.TB) *sagaredis.Store {
	tb.Helper()
	client, _ := testhelpers.RedisClient(tb)
	return sagaredis.New(client)
}

func instance(id string, status saga.Status, deadline time.Time) *saga.Instance {
	return &saga.Instance{
		ID:         id,
		Definition: "place-order",
		Status:     status,
		Stage:      1,
		Data:       []byte(`{"n":1}`),
		CreatedAt:  baseTime,
		UpdatedAt:  baseTime,
		Deadline:   deadline,
	}
}

func TestCreateAndGet(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()

	inst := instance("a", saga.StatusRunning, time.Time{})
	require.NoError(t, s.Create(ctx, inst))

	got, err := s.Get(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, inst, got)
}

func TestGetMissing(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	_, err := s.Get(t.Context(), "missing")
	require.ErrorIs(t, err, sagaerrs.ErrInstanceNotFound)
}

func TestCreateDuplicate(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()

	inst := instance("a", saga.StatusRunning, time.Time{})
	require.NoError(t, s.Create(ctx, inst))
	require.ErrorIs(t, s.Create(ctx, inst), sagaerrs.ErrInstanceExists)
}

func TestUpdateVersionCAS(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()

	inst := instance("a", saga.StatusRunning, time.Time{})
	require.NoError(t, s.Create(ctx, inst)) // version 0

	inst.Stage = 2
	require.NoError(t, s.Update(ctx, inst))
	require.Equal(t, int64(1), inst.Version)

	// A stale writer (still at version 0) must lose.
	stale := instance("a", saga.StatusRunning, time.Time{})
	stale.Version = 0
	require.ErrorIs(t, s.Update(ctx, stale), sagaerrs.ErrVersionConflict)

	got, err := s.Get(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, 2, got.Stage)
	require.Equal(t, int64(1), got.Version)
}

func TestUpdateMissing(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	require.ErrorIs(t, s.Update(t.Context(), instance("ghost", saga.StatusRunning, time.Time{})), sagaerrs.ErrInstanceNotFound)
}

func TestDelete(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()

	require.NoError(t, s.Create(ctx, instance("a", saga.StatusCompensating, time.Time{})))
	require.NoError(t, s.Delete(ctx, "a"))

	_, err := s.Get(ctx, "a")
	require.ErrorIs(t, err, sagaerrs.ErrInstanceNotFound)

	// Deleting a missing instance is not an error, and it clears the index.
	require.NoError(t, s.Delete(ctx, "a"))
	rec, err := s.FetchRecoverable(ctx, baseTime.Add(time.Hour), 0)
	require.NoError(t, err)
	require.Empty(t, rec)
}

func TestFetchRecoverable(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()

	past := baseTime.Add(-time.Minute)
	future := baseTime.Add(time.Hour)

	require.NoError(t, s.Create(ctx, instance("timed-out", saga.StatusRunning, past)))
	require.NoError(t, s.Create(ctx, instance("future", saga.StatusRunning, future)))
	require.NoError(t, s.Create(ctx, instance("no-deadline", saga.StatusRunning, time.Time{})))
	require.NoError(t, s.Create(ctx, instance("compensating", saga.StatusCompensating, time.Time{})))
	require.NoError(t, s.Create(ctx, instance("done", saga.StatusCompleted, time.Time{})))

	rec, err := s.FetchRecoverable(ctx, baseTime, 0)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"timed-out", "compensating"}, ids(rec))
}

func TestFetchRecoverableRespectsLimit(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()

	require.NoError(t, s.Create(ctx, instance("c1", saga.StatusCompensating, time.Time{})))
	require.NoError(t, s.Create(ctx, instance("c2", saga.StatusCompensating, time.Time{})))

	rec, err := s.FetchRecoverable(ctx, baseTime, 1)
	require.NoError(t, err)
	require.Len(t, rec, 1)
}

func TestUpdateRemovesTerminalFromIndex(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()

	inst := instance("a", saga.StatusRunning, baseTime.Add(-time.Minute))
	require.NoError(t, s.Create(ctx, inst))

	rec, err := s.FetchRecoverable(ctx, baseTime, 0)
	require.NoError(t, err)
	require.Len(t, rec, 1)

	inst.Status = saga.StatusCompleted
	require.NoError(t, s.Update(ctx, inst))

	rec, err = s.FetchRecoverable(ctx, baseTime, 0)
	require.NoError(t, err)
	require.Empty(t, rec)
}

func ids(insts []*saga.Instance) []string {
	out := make([]string, len(insts))
	for i, in := range insts {
		out[i] = in.ID
	}
	return out
}
