// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/data/saga/storages/memory"

	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
)

func TestCreateGetDelete(t *testing.T) {
	t.Parallel()
	s := memory.New()
	ctx := t.Context()

	require.NoError(t, s.Create(ctx, &saga.Instance{ID: "a", Status: saga.StatusRunning}))
	require.Equal(t, 1, s.Len())

	got, err := s.Get(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, "a", got.ID)

	require.NoError(t, s.Delete(ctx, "a"))
	require.Equal(t, 0, s.Len())
	require.NoError(t, s.Delete(ctx, "a")) // deleting missing is not an error

	_, err = s.Get(ctx, "missing")
	require.ErrorIs(t, err, sagaerrs.ErrInstanceNotFound)
}

func TestCreateDuplicate(t *testing.T) {
	t.Parallel()
	s := memory.New()
	ctx := t.Context()
	require.NoError(t, s.Create(ctx, &saga.Instance{ID: "a"}))
	require.ErrorIs(t, s.Create(ctx, &saga.Instance{ID: "a"}), sagaerrs.ErrInstanceExists)
}

func TestUpdateVersionCAS(t *testing.T) {
	t.Parallel()
	s := memory.New()
	ctx := t.Context()
	require.NoError(t, s.Create(ctx, &saga.Instance{ID: "a", Status: saga.StatusRunning}))

	a, err := s.Get(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, int64(0), a.Version)

	a.Stage = 1
	require.NoError(t, s.Update(ctx, a))
	require.Equal(t, int64(1), a.Version) // version written back on success

	stale := &saga.Instance{ID: "a", Version: 0}
	require.ErrorIs(t, s.Update(ctx, stale), sagaerrs.ErrVersionConflict)

	require.ErrorIs(t, s.Update(ctx, &saga.Instance{ID: "missing"}), sagaerrs.ErrInstanceNotFound)
}

func TestFetchRecoverable(t *testing.T) {
	t.Parallel()
	s := memory.New()
	ctx := t.Context()
	now := time.Now().UTC()

	require.NoError(t, s.Create(ctx, &saga.Instance{ID: "running-ok", Status: saga.StatusRunning}))
	require.NoError(t, s.Create(ctx, &saga.Instance{ID: "running-timed-out", Status: saga.StatusRunning, Deadline: now.Add(-time.Minute)}))
	require.NoError(t, s.Create(ctx, &saga.Instance{ID: "compensating", Status: saga.StatusCompensating}))
	require.NoError(t, s.Create(ctx, &saga.Instance{ID: "completed", Status: saga.StatusCompleted}))

	got, err := s.FetchRecoverable(ctx, now, 0)
	require.NoError(t, err)

	ids := map[string]bool{}
	for _, inst := range got {
		ids[inst.ID] = true
	}
	require.True(t, ids["running-timed-out"])
	require.True(t, ids["compensating"])
	require.False(t, ids["running-ok"])
	require.False(t, ids["completed"])

	// Limit is respected.
	limited, err := s.FetchRecoverable(ctx, now, 1)
	require.NoError(t, err)
	require.Len(t, limited, 1)
}

func TestStoredCopiesAreIsolated(t *testing.T) {
	t.Parallel()
	s := memory.New()
	ctx := t.Context()

	orig := &saga.Instance{ID: "a", Data: []byte("v1")}
	require.NoError(t, s.Create(ctx, orig))

	orig.Data[0] = 'X' // mutate after Create — must not affect the stored copy

	got, err := s.Get(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, []byte("v1"), got.Data)

	got.Data[0] = 'Y' // mutate the returned copy — must not affect the store
	again, err := s.Get(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, []byte("v1"), again.Data)
}
