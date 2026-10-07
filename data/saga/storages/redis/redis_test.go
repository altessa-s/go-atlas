// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	goredis "github.com/redis/go-redis/v9"

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
	rec, err := s.FetchRecoverable(ctx, "", baseTime.Add(time.Hour), 0)
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

	rec, err := s.FetchRecoverable(ctx, "", baseTime, 0)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"timed-out", "compensating"}, ids(rec))
}

func TestFetchRecoverableRespectsLimit(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()

	require.NoError(t, s.Create(ctx, instance("c1", saga.StatusCompensating, time.Time{})))
	require.NoError(t, s.Create(ctx, instance("c2", saga.StatusCompensating, time.Time{})))

	rec, err := s.FetchRecoverable(ctx, "", baseTime, 1)
	require.NoError(t, err)
	require.Len(t, rec, 1)
}

// removeAfterFirstPipeline is a go-redis hook that, once the first instance
// page has been loaded, removes the given members from the recoverable index —
// a deterministic stand-in for concurrent writers finishing those sagas while
// FetchRecoverable is mid-scan.
type removeAfterFirstPipeline struct {
	mr      *miniredis.Miniredis
	members []string
	done    bool
}

func (h *removeAfterFirstPipeline) DialHook(next goredis.DialHook) goredis.DialHook { return next }

func (h *removeAfterFirstPipeline) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return next
}

func (h *removeAfterFirstPipeline) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []goredis.Cmder) error {
		err := next(ctx, cmds)
		if !h.done {
			h.done = true
			for _, key := range h.mr.Keys() {
				if strings.HasSuffix(key, "index:recoverable") {
					for _, m := range h.members {
						_, _ = h.mr.ZRem(key, m)
					}
				}
			}
		}
		return err
	}
}

// TestFetchRecoverableStableUnderConcurrentRemoval pins that entries removed
// from the index while FetchRecoverable is between pages do not shift later
// entries out of the scan.
func TestFetchRecoverableStableUnderConcurrentRemoval(t *testing.T) {
	t.Parallel()
	client, mr := testhelpers.RedisClient(t)
	s := sagaredis.New(client)
	ctx := t.Context()

	var first []string
	for i := range 64 {
		id := fmt.Sprintf("a-%02d", i)
		other := instance(id, saga.StatusCompensating, time.Time{})
		other.Definition = "ship-order"
		require.NoError(t, s.Create(ctx, other))
		first = append(first, id)
	}
	for i := range 2 {
		require.NoError(t, s.Create(ctx, instance(fmt.Sprintf("z-%d", i), saga.StatusCompensating, time.Time{})))
	}

	client.AddHook(&removeAfterFirstPipeline{mr: mr, members: first})

	rec, err := s.FetchRecoverable(ctx, "place-order", baseTime, 2)
	require.NoError(t, err)
	got := make([]string, 0, len(rec))
	for _, inst := range rec {
		got = append(got, inst.ID)
	}
	require.Equal(t, []string{"z-0", "z-1"}, got)
}

// TestFetchRecoverableSkipsNonMatchingPages pins that entries which do not
// count — another definition's instances, or own instances under an active
// lease — never fill the batch, even when they span more than one index page.
// All entries score 0 (COMPENSATING), so the index orders them by member and
// the "a-" / "b-" entries come before the own "z-" ones.
func TestFetchRecoverableSkipsNonMatchingPages(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()

	for i := range 35 {
		other := instance(fmt.Sprintf("a-%02d", i), saga.StatusCompensating, time.Time{})
		other.Definition = "ship-order"
		require.NoError(t, s.Create(ctx, other))

		leased := instance(fmt.Sprintf("b-%02d", i), saga.StatusCompensating, time.Time{})
		leased.LeaseOwner, leased.LeaseUntil = "owner", baseTime.Add(time.Hour)
		require.NoError(t, s.Create(ctx, leased))
	}
	for i := range 2 {
		require.NoError(t, s.Create(ctx, instance(fmt.Sprintf("z-%d", i), saga.StatusCompensating, time.Time{})))
	}

	rec, err := s.FetchRecoverable(ctx, "place-order", baseTime, 2)
	require.NoError(t, err)
	got := make([]string, 0, len(rec))
	for _, inst := range rec {
		got = append(got, inst.ID)
	}
	require.Equal(t, []string{"z-0", "z-1"}, got)
}

func TestUpdateRemovesTerminalFromIndex(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()

	inst := instance("a", saga.StatusRunning, baseTime.Add(-time.Minute))
	require.NoError(t, s.Create(ctx, inst))

	rec, err := s.FetchRecoverable(ctx, "", baseTime, 0)
	require.NoError(t, err)
	require.Len(t, rec, 1)

	inst.Status = saga.StatusCompleted
	require.NoError(t, s.Update(ctx, inst))

	rec, err = s.FetchRecoverable(ctx, "", baseTime, 0)
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

func TestLeaseExcludesExpiredDeadline(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	inst := &saga.Instance{ID: "lease", Definition: "test", Status: saga.StatusRunning, Deadline: now.Add(-time.Minute), LeaseOwner: "owner", LeaseUntil: now.Add(time.Minute), PendingSteps: []int{0, 1}}
	require.NoError(t, store.Create(t.Context(), inst))
	got, err := store.Get(t.Context(), inst.ID)
	require.NoError(t, err)
	require.Equal(t, inst.PendingSteps, got.PendingSteps)
	require.Equal(t, inst.LeaseUntil, got.LeaseUntil)
	records, err := store.FetchRecoverable(t.Context(), "", now, 10)
	require.NoError(t, err)
	require.Empty(t, records)
	records, err = store.FetchRecoverable(t.Context(), "", now.Add(2*time.Minute), 10)
	require.NoError(t, err)
	require.Len(t, records, 1)
	inst.Deadline = time.Time{}
	inst.LeaseUntil = now.Add(-time.Second)
	require.NoError(t, store.Update(t.Context(), inst))
	records, err = store.FetchRecoverable(t.Context(), "", now, 10)
	require.NoError(t, err)
	require.Len(t, records, 1)
}
