// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storagetest

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/saga"

	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
)

// contracts lists the storage contracts; [Run] executes each.
var contracts = []struct {
	name  string
	check func(*testing.T, saga.Storage)
}{
	{"RoundTrip", RoundTrip},
	{"CreateExisting", CreateExisting},
	{"ConcurrentCreate", ConcurrentCreate},
	{"Update", Update},
	{"ConcurrentUpdate", ConcurrentUpdate},
	{"Delete", Delete},
	{"Identity", Identity},
	{"ExactIdentity", ExactIdentity},
	{"FetchRecoverable", FetchRecoverable},
	{"FetchRecoverableBoundaries", FetchRecoverableBoundaries},
	{"FetchRecoverableLimit", FetchRecoverableLimit},
}

// Run executes every storage contract as a parallel subtest named after it,
// each on a fresh store from newStore, which must return an empty store
// isolated from every other store it returns. The contracts named in skip are
// reported as skipped; a backend names only those its medium cannot satisfy,
// such as [ExactIdentity] for a key-value store with a restricted key alphabet.
func Run(t *testing.T, newStore func(testing.TB) saga.Storage, skip ...string) {
	t.Helper()
	for _, c := range contracts {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if slices.Contains(skip, c.name) {
				t.Skip("not supported by this backend")
			}
			c.check(t, newStore(t))
		})
	}
}

// base is the fixtures' reference instant: whole seconds, UTC, and free of a
// monotonic reading, so round-tripped values compare with require.Equal.
var base = time.Unix(1_700_000_000, 0).UTC() //nolint:mnd // A fixed reference instant.

// Granularity is how late a store may report an expired lease or deadline.
const Granularity = time.Second

// concurrency is the number of goroutines racing in the concurrent contracts.
const concurrency = 16

// instance builds a minimal instance with whole-second timestamps.
func instance(id string, status saga.Status) *saga.Instance {
	return &saga.Instance{
		ID:         id,
		Definition: "place-order",
		Status:     status,
		CreatedAt:  base,
		UpdatedAt:  base,
	}
}

// full builds an instance with every field set.
//
//nolint:mnd // Fixed values describe the storage contract.
func full(id string) *saga.Instance {
	inst := instance(id, saga.StatusRunning)
	inst.Stage = 2
	inst.PendingSteps = []int{0, 3}
	inst.Data = []byte{0, 1, 2, 0xff, '"', '\'', '\\'}
	inst.Steps = []saga.StepRecord{
		{Name: "reserve", Stage: 0, Status: saga.StepCompleted, Attempts: 1, StartedAt: base, FinishedAt: base.Add(time.Second)},
		{Name: "charge", Stage: 1, Status: saga.StepFailed, Attempts: 3, Error: "card declined: \"insufficient\" ü", StartedAt: base.Add(2 * time.Second)},
	}
	inst.UpdatedAt = base.Add(time.Minute)
	inst.Deadline = base.Add(time.Hour)
	inst.LeaseOwner = "owner-ü"
	inst.LeaseUntil = base.Add(30*time.Second + 123456789*time.Nanosecond)
	inst.LastError = "boom\nline two"
	return inst
}

// requireSame asserts that got equals want in every field but the opaque
// version, treating nil and empty slices alike.
func requireSame(t *testing.T, want, got *saga.Instance) {
	t.Helper()
	require.NotNil(t, got)
	w, g := want.Clone(), got.Clone()
	g.Version = w.Version
	for _, inst := range []*saga.Instance{w, g} {
		if len(inst.PendingSteps) == 0 {
			inst.PendingSteps = nil
		}
		if len(inst.Data) == 0 {
			inst.Data = nil
		}
		if len(inst.Steps) == 0 {
			inst.Steps = nil
		}
	}
	require.Equal(t, w, g)
}

// ids returns the IDs of insts as a set.
func ids(insts []*saga.Instance) map[string]bool {
	out := make(map[string]bool, len(insts))
	for _, inst := range insts {
		out[inst.ID] = true
	}
	return out
}

// RoundTrip verifies that every field survives Create and Get — LeaseUntil at
// full precision, Data as opaque bytes — that a zero-valued instance round-trips
// too, and that neither the caller's instance nor a returned one aliases the
// stored state.
func RoundTrip(t *testing.T, store saga.Storage) {
	t.Helper()
	ctx := t.Context()

	inst := full("full")
	want := inst.Clone()
	require.NoError(t, store.Create(ctx, inst))
	inst.Data[0] = 'X'
	inst.Steps[0].Name = "mutated"
	inst.PendingSteps[0] = 9

	got, err := store.Get(ctx, "full")
	require.NoError(t, err)
	requireSame(t, want, got)

	got.Data[0] = 'Y'
	got.Steps[0].Name = "mutated"
	got.PendingSteps[0] = 9
	again, err := store.Get(ctx, "full")
	require.NoError(t, err)
	requireSame(t, want, again)

	zero := &saga.Instance{ID: "zero"}
	require.NoError(t, store.Create(ctx, zero))
	got, err = store.Get(ctx, "zero")
	require.NoError(t, err)
	requireSame(t, &saga.Instance{ID: "zero"}, got)

	_, err = store.Get(ctx, "missing")
	require.ErrorIs(t, err, sagaerrs.ErrInstanceNotFound)
}

// CreateExisting verifies that Create on a stored ID fails with
// [sagaerrs.ErrInstanceExists] and leaves the stored instance untouched.
func CreateExisting(t *testing.T, store saga.Storage) {
	t.Helper()
	ctx := t.Context()

	first := full("a")
	want := first.Clone()
	require.NoError(t, store.Create(ctx, first))

	second := instance("a", saga.StatusCompensating)
	require.ErrorIs(t, store.Create(ctx, second), sagaerrs.ErrInstanceExists)

	got, err := store.Get(ctx, "a")
	require.NoError(t, err)
	requireSame(t, want, got)
}

// ConcurrentCreate verifies that exactly one of many concurrent creators of
// the same ID wins and every other one gets [sagaerrs.ErrInstanceExists].
func ConcurrentCreate(t *testing.T, store saga.Storage) {
	t.Helper()
	ctx := t.Context()

	results := make([]error, concurrency)
	var wg sync.WaitGroup
	for i := range concurrency {
		wg.Go(func() {
			inst := instance("race", saga.StatusRunning)
			inst.LastError = fmt.Sprintf("creator-%d", i)
			results[i] = store.Create(ctx, inst)
		})
	}
	wg.Wait()

	winner := -1
	for i, err := range results {
		if err == nil {
			require.Equal(t, -1, winner, "more than one concurrent Create won")
			winner = i
			continue
		}
		require.ErrorIs(t, err, sagaerrs.ErrInstanceExists)
	}
	require.NotEqual(t, -1, winner, "no concurrent Create won")

	got, err := store.Get(ctx, "race")
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("creator-%d", winner), got.LastError)
}

// Update verifies the version compare-and-swap: a successful Update persists
// every mutable field, changes the version and writes it back; a stale version
// fails with [sagaerrs.ErrVersionConflict] and a missing ID with
// [sagaerrs.ErrInstanceNotFound], both leaving the store and the caller's
// version untouched.
//
//nolint:mnd // Fixed values describe the storage contract.
func Update(t *testing.T, store saga.Storage) {
	t.Helper()
	ctx := t.Context()

	require.NoError(t, store.Create(ctx, full("a")))
	cur, err := store.Get(ctx, "a")
	require.NoError(t, err)
	stale := cur.Clone()

	cur.Status = saga.StatusCompensating
	cur.Stage = 1
	cur.PendingSteps = nil
	cur.Data = []byte("v2")
	cur.Steps = append(cur.Steps, saga.StepRecord{Name: "ship", Stage: 2, Status: saga.StepPending})
	cur.UpdatedAt = base.Add(2 * time.Minute)
	cur.Deadline = time.Time{}
	cur.LeaseOwner = ""
	cur.LeaseUntil = time.Time{}
	cur.LastError = ""
	oldVersion := cur.Version
	require.NoError(t, store.Update(ctx, cur))
	require.NotEqual(t, oldVersion, cur.Version, "Update must write the new version back")

	got, err := store.Get(ctx, "a")
	require.NoError(t, err)
	requireSame(t, cur, got)
	require.Equal(t, cur.Version, got.Version, "Get must return the version Update wrote back")

	stale.Status = saga.StatusCompleted
	staleVersion := stale.Version
	require.ErrorIs(t, store.Update(ctx, stale), sagaerrs.ErrVersionConflict)
	require.Equal(t, staleVersion, stale.Version, "a failed Update must not change the caller's version")
	got, err = store.Get(ctx, "a")
	require.NoError(t, err)
	requireSame(t, cur, got)

	// The written-back version is current: a second Update succeeds.
	cur.Stage = 0
	require.NoError(t, store.Update(ctx, cur))

	ghost := instance("ghost", saga.StatusRunning)
	require.ErrorIs(t, store.Update(ctx, ghost), sagaerrs.ErrInstanceNotFound)
	_, err = store.Get(ctx, "ghost")
	require.ErrorIs(t, err, sagaerrs.ErrInstanceNotFound, "Update must not create a missing instance")
}

// ConcurrentUpdate verifies that exactly one of many concurrent Updates from
// the same version wins and every other one gets [sagaerrs.ErrVersionConflict].
func ConcurrentUpdate(t *testing.T, store saga.Storage) {
	t.Helper()
	ctx := t.Context()

	require.NoError(t, store.Create(ctx, instance("race", saga.StatusRunning)))
	read, err := store.Get(ctx, "race")
	require.NoError(t, err)

	results := make([]error, concurrency)
	var wg sync.WaitGroup
	for i := range concurrency {
		wg.Go(func() {
			inst := read.Clone()
			inst.LastError = fmt.Sprintf("writer-%d", i)
			results[i] = store.Update(ctx, inst)
		})
	}
	wg.Wait()

	winner := -1
	for i, err := range results {
		if err == nil {
			require.Equal(t, -1, winner, "more than one concurrent Update won")
			winner = i
			continue
		}
		require.ErrorIs(t, err, sagaerrs.ErrVersionConflict)
	}
	require.NotEqual(t, -1, winner, "no concurrent Update won")

	got, err := store.Get(ctx, "race")
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("writer-%d", winner), got.LastError)
}

// Delete verifies that Delete removes an instance — Get and FetchRecoverable
// no longer see it — and that deleting a missing one is not an error.
func Delete(t *testing.T, store saga.Storage) {
	t.Helper()
	ctx := t.Context()

	require.NoError(t, store.Create(ctx, instance("a", saga.StatusCompensating)))
	require.NoError(t, store.Create(ctx, instance("b", saga.StatusCompensating)))
	require.NoError(t, store.Delete(ctx, "a"))

	_, err := store.Get(ctx, "a")
	require.ErrorIs(t, err, sagaerrs.ErrInstanceNotFound)
	_, err = store.Get(ctx, "b")
	require.NoError(t, err, "Delete must remove only the given instance")

	require.NoError(t, store.Delete(ctx, "a"))
	require.NoError(t, store.Delete(ctx, "never"))

	rec, err := store.FetchRecoverable(ctx, base.Add(time.Hour), 0)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"b": true}, ids(rec))

	// A deleted ID can be created again.
	require.NoError(t, store.Create(ctx, instance("a", saga.StatusRunning)))
}

// Identity verifies that IDs differing only by case are distinct instances.
func Identity(t *testing.T, store saga.Storage) {
	t.Helper()
	requireDistinct(t, store, []string{"order", "Order", "ORDER", "order-1"})
}

// ExactIdentity verifies that IDs compare byte for byte: an ID with a trailing
// space is distinct from the one without, and non-ASCII IDs and IDs with quotes
// round-trip.
func ExactIdentity(t *testing.T, store saga.Storage) {
	t.Helper()
	requireDistinct(t, store, []string{"order", "order ", "заказ-ü", "id'with\"quotes"})

	require.NoError(t, store.Delete(t.Context(), "order "))
	_, err := store.Get(t.Context(), "order")
	require.NoError(t, err, `deleting "order " must not delete "order"`)
}

// requireDistinct creates one instance per ID and checks that each reads back
// as its own.
func requireDistinct(t *testing.T, store saga.Storage, keys []string) {
	t.Helper()
	ctx := t.Context()
	for _, id := range keys {
		inst := instance(id, saga.StatusRunning)
		inst.LastError = "for " + id
		require.NoError(t, store.Create(ctx, inst), id)
	}
	for _, id := range keys {
		got, err := store.Get(ctx, id)
		require.NoError(t, err, id)
		require.Equal(t, id, got.ID)
		require.Equal(t, "for "+id, got.LastError)
	}
}

// FetchRecoverable verifies the recovery predicate of [saga.Instance.Recoverable]:
// interrupted compensation, an abandoned lease and an expired deadline qualify;
// terminal instances, healthy ones and every instance under an active lease do
// not.
func FetchRecoverable(t *testing.T, store saga.Storage) {
	t.Helper()
	ctx := t.Context()

	now := base.Add(time.Hour)
	past, future := now.Add(-time.Minute), now.Add(time.Minute)
	create := func(id string, status saga.Status, deadline time.Time, owner string, leaseUntil time.Time) {
		inst := instance(id, status)
		inst.Deadline, inst.LeaseOwner, inst.LeaseUntil = deadline, owner, leaseUntil
		require.NoError(t, store.Create(ctx, inst), id)
	}

	create("compensating", saga.StatusCompensating, time.Time{}, "", time.Time{})
	create("timed-out", saga.StatusRunning, past, "", time.Time{})
	create("abandoned", saga.StatusRunning, future, "owner", past)
	create("zero-lease", saga.StatusRunning, time.Time{}, "owner", time.Time{})
	create("expired-lease-compensating", saga.StatusCompensating, time.Time{}, "owner", past)

	create("healthy", saga.StatusRunning, future, "", time.Time{})
	create("no-deadline", saga.StatusRunning, time.Time{}, "", time.Time{})
	create("active-lease", saga.StatusCompensating, past, "owner", future)
	create("active-lease-timed-out", saga.StatusRunning, past, "owner", future)
	create("lease-without-owner", saga.StatusRunning, time.Time{}, "", past)
	for _, status := range []saga.Status{saga.StatusCompleted, saga.StatusCompensated, saga.StatusFailed} {
		create("terminal-"+string(status), status, past, "owner", past)
	}

	rec, err := store.FetchRecoverable(ctx, now, 0)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{
		"compensating": true, "timed-out": true, "abandoned": true, "zero-lease": true, "expired-lease-compensating": true,
	}, ids(rec))
	for _, inst := range rec {
		require.True(t, inst.Recoverable(now), inst.ID)
	}
}

// FetchRecoverableBoundaries verifies the predicate's boundaries. A lease
// ending a nanosecond after now is still active and must not be returned —
// a store that rounds LeaseUntil would hand out a live execution. An expired
// lease may be reported up to [Granularity] late, which a store indexing
// leases by the second needs; a deadline at now has passed while one a second
// later has not.
func FetchRecoverableBoundaries(t *testing.T, store saga.Storage) {
	t.Helper()
	ctx := t.Context()

	now := base.Add(time.Hour + 500*time.Millisecond)
	lease := func(id string, until time.Time) {
		inst := instance(id, saga.StatusRunning)
		inst.LeaseOwner, inst.LeaseUntil = "owner", until
		require.NoError(t, store.Create(ctx, inst), id)
	}
	lease("lease-at-now", now)
	lease("lease-after-now", now.Add(time.Nanosecond))
	lease("lease-before-now", now.Add(-time.Nanosecond))

	wholeNow := now.Truncate(time.Second)
	deadline := func(id string, at time.Time) {
		inst := instance(id, saga.StatusRunning)
		inst.Deadline = at
		require.NoError(t, store.Create(ctx, inst), id)
	}
	deadline("deadline-at-now", wholeNow)
	deadline("deadline-after-now", wholeNow.Add(time.Second))

	rec, err := store.FetchRecoverable(ctx, wholeNow, 0)
	require.NoError(t, err)
	got := ids(rec)
	require.True(t, got["deadline-at-now"], "a deadline at now has passed")
	require.False(t, got["deadline-after-now"], "a deadline after now has not passed")

	rec, err = store.FetchRecoverable(ctx, now, 0)
	require.NoError(t, err)
	for _, inst := range rec {
		require.True(t, inst.Recoverable(now), "%s is not recoverable at now", inst.ID)
	}
	require.False(t, ids(rec)["lease-after-now"], "a lease ending a nanosecond after now is still active")

	rec, err = store.FetchRecoverable(ctx, now.Add(Granularity), 0)
	require.NoError(t, err)
	got = ids(rec)
	require.True(t, got["lease-at-now"], "a lease ending at now has expired")
	require.True(t, got["lease-before-now"], "a lease ending before now has expired")
	require.True(t, got["lease-after-now"], "a lease ending a nanosecond after now has expired a second later")
}

// FetchRecoverableLimit verifies that a positive limit caps the result and a
// non-positive one does not.
//
//nolint:mnd // Fixed counts describe the storage contract.
func FetchRecoverableLimit(t *testing.T, store saga.Storage) {
	t.Helper()
	ctx := t.Context()

	for i := range 5 {
		require.NoError(t, store.Create(ctx, instance(fmt.Sprintf("c%d", i), saga.StatusCompensating)))
	}
	rec, err := store.FetchRecoverable(ctx, base, 2)
	require.NoError(t, err)
	require.NotEmpty(t, rec)
	require.LessOrEqual(t, len(rec), 2)

	for _, limit := range []int{0, -1} {
		rec, err = store.FetchRecoverable(ctx, base, limit)
		require.NoError(t, err)
		require.Len(t, rec, 5, "limit %d", limit)
	}
}
