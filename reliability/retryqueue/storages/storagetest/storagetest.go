// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storagetest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/retryqueue"
)

// Payload is the caller data the contracts store. Tag is omitted from the
// encoding when empty, so [ReopenReplacesPayload] can check that a reopen
// drops a field the new payload leaves out.
type Payload struct {
	ValueOn time.Time `bson:"valueOn"`
	Note    string    `bson:"note"`
	Tag     string    `bson:"tag,omitempty"`
}

// Storage is the storage type the contracts exercise.
type Storage = retryqueue.Storage[Payload]

// Item is the item type the contracts exercise.
type Item = retryqueue.Item[Payload]

// contracts lists the storage contracts; [Run] executes each.
var contracts = []struct {
	name  string
	check func(*testing.T, Storage)
}{
	{"MarkIsInsertOnly", MarkIsInsertOnly},
	{"MarkStoresFreshItem", MarkStoresFreshItem},
	{"ListDueOrderAndReschedule", ListDueOrderAndReschedule},
	{"ListDueLimit", ListDueLimit},
	{"ExpireKeepsItemButStopsListing", ExpireKeepsItemButStopsListing},
	{"DeleteIsIdempotent", DeleteIsIdempotent},
	{"ResolveKeepsPinnedPayload", ResolveKeepsPinnedPayload},
	{"ResolveIsIdempotentAndClearsExpiry", ResolveIsIdempotentAndClearsExpiry},
	{"WakeDueMakesPendingDue", WakeDueMakesPendingDue},
	{"FlagOnce", FlagOnce},
	{"FlagReasonChangeAndClear", FlagReasonChangeAndClear},
	{"FlagOnlyPending", FlagOnlyPending},
	{"RequeueRevivesExpired", RequeueRevivesExpired},
	{"ReopenMovesToNewerGenerationOnly", ReopenMovesToNewerGenerationOnly},
	{"ReopenSameGeneration", ReopenSameGeneration},
	{"ReopenMissingInserts", ReopenMissingInserts},
	{"ReopenReplacesPayload", ReopenReplacesPayload},
	{"StaleExpireSparesRequeued", StaleExpireSparesRequeued},
	{"StaleGenerationIsNoOp", StaleGenerationIsNoOp},
	{"ListByStateOrder", ListByStateOrder},
	{"MissingItemIsNoOp", MissingItemIsNoOp},
	{"ConcurrentMark", ConcurrentMark},
	{"ConcurrentExpire", ConcurrentExpire},
	{"ConcurrentFlag", ConcurrentFlag},
	{"MoveKeepsEveryField", MoveKeepsEveryField},
	{"MoveRefusals", MoveRefusals},
	{"MoveOntoItself", MoveOntoItself},
	{"ConcurrentMoveFromOneKey", ConcurrentMoveFromOneKey},
	{"ConcurrentMoveOntoOneKey", ConcurrentMoveOntoOneKey},
	{"DoneContextChangesNothing", DoneContextChangesNothing},
}

// Run executes every storage contract as a parallel subtest named after it,
// each on a fresh store from newStore, which must return an empty store
// isolated from every other store it returns. The contracts named in skip are
// reported as skipped, and so are the Move contracts when the store does not
// implement [retryqueue.Mover].
func Run(t *testing.T, newStore func(testing.TB) Storage, skip ...string) {
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

// T0 is the fixtures' reference instant: whole seconds, UTC, and free of a
// monotonic reading, so round-tripped values compare with require.Equal. It is
// the current time, not a fixed date: a backend that purges by its own clock
// (a TTL on resolved items) would delete an item resolved at a date long past
// in the middle of a contract.
var T0 = time.Now().UTC().Truncate(time.Second)

// concurrency is the number of goroutines racing in the concurrent contracts.
const concurrency = 16

// Pending returns a generation-1 item of key due at next, first seen at [T0].
func Pending(key string, next time.Time) Item {
	return Item{
		Key: key, Generation: 1, Since: T0, NextAttemptAt: next,
		Payload: Payload{ValueOn: T0.Add(-time.Hour), Note: "first"},
	}
}

// mark stores items with Mark, failing the test on an error.
func mark(t *testing.T, s Storage, items ...Item) {
	t.Helper()
	for _, it := range items {
		_, err := s.Mark(t.Context(), it)
		require.NoError(t, err, "mark %s", it.Key)
	}
}

// read returns the stored item of key through an insert-only Mark.
func read(t *testing.T, s Storage, key string) Item {
	t.Helper()
	got, err := s.Mark(t.Context(), Pending(key, T0))
	require.NoError(t, err)
	return got
}

// keys returns the keys of items in order.
func keys(items []Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Key)
	}
	return out
}

// due lists the items due at now with a generous limit.
//
//nolint:mnd // Fixed instants, generations and counts describe the storage contract.
func due(t *testing.T, s Storage, now time.Time) []Item {
	t.Helper()
	got, err := s.ListDue(t.Context(), now, 100)
	require.NoError(t, err)
	return got
}

// MarkIsInsertOnly asserts a repeated Mark keeps the original age, schedule
// and payload.
//
//nolint:mnd // Fixed instants, generations and counts describe the storage contract.
func MarkIsInsertOnly(t *testing.T, s Storage) {
	first, err := s.Mark(t.Context(), Pending("k1", T0))
	require.NoError(t, err)

	again := Pending("k1", T0.Add(time.Hour))
	again.Since = T0.Add(24 * time.Hour)
	again.Payload = Payload{ValueOn: T0.Add(24 * time.Hour), Note: "second"} //nolint:goconst // each contract test labels its own payloads
	got, err := s.Mark(t.Context(), again)
	require.NoError(t, err)
	require.Equal(t, first, got)
	require.Equal(t, T0, got.NextAttemptAt)
	require.Equal(t, Payload{ValueOn: T0.Add(-time.Hour), Note: "first"}, got.Payload)
}

// MarkStoresFreshItem asserts Mark stores a pending item with no attempts,
// flag, expiry or resolution whatever the argument carries.
//
//nolint:mnd // Fixed instants, generations and counts describe the storage contract.
func MarkStoresFreshItem(t *testing.T, s Storage) {
	in := Pending("k1", T0)
	in.Generation = 7
	in.State = retryqueue.StateExpired
	in.Attempts = 3
	in.ExpiredAt = T0
	in.ResolvedAt = T0
	in.Flagged, in.FlagReason = true, "why"
	got, err := s.Mark(t.Context(), in)
	require.NoError(t, err)
	require.Equal(t, Item{
		Key: "k1", Generation: 7, State: retryqueue.StatePending, Since: T0, NextAttemptAt: T0, Payload: in.Payload,
	}, got)
}

// ListDueOrderAndReschedule asserts the due order (next attempt, then key) and
// that Reschedule counts an attempt and moves the item out of the due list.
//
//nolint:mnd // Fixed instants, generations and counts describe the storage contract.
func ListDueOrderAndReschedule(t *testing.T, s Storage) {
	mark(t, s, Pending("b", T0), Pending("a", T0), Pending("c", T0.Add(-time.Minute)), Pending("later", T0.Add(time.Hour)))
	require.Equal(t, []string{"c", "a", "b"}, keys(due(t, s, T0)))

	require.NoError(t, s.Reschedule(t.Context(), "c", 1, T0.Add(2*time.Hour)))
	require.Equal(t, []string{"a", "b"}, keys(due(t, s, T0)))

	got := due(t, s, T0.Add(3*time.Hour))
	require.Equal(t, []string{"a", "b", "later", "c"}, keys(got))
	require.Equal(t, 1, got[3].Attempts)
	require.Equal(t, T0.Add(2*time.Hour), got[3].NextAttemptAt)
}

// ListDueLimit asserts the limit, and that a limit that is not positive means
// the default.
//
//nolint:mnd // Fixed instants, generations and counts describe the storage contract.
func ListDueLimit(t *testing.T, s Storage) {
	mark(t, s, Pending("a", T0), Pending("b", T0), Pending("c", T0))
	got, err := s.ListDue(t.Context(), T0, 2)
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, keys(got))
	for _, limit := range []int{0, -1} {
		got, err = s.ListDue(t.Context(), T0, limit)
		require.NoError(t, err)
		require.Len(t, got, 3)
	}
	got, err = s.ListDue(t.Context(), T0.Add(-time.Hour), 0)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Empty(t, got)
}

// ExpireKeepsItemButStopsListing asserts an expired item is no longer due, is
// never revived by Reschedule, and a repeated Mark returns it as is.
func ExpireKeepsItemButStopsListing(t *testing.T, s Storage) {
	mark(t, s, Pending("k9", T0))
	ok, err := s.Expire(t.Context(), "k9", 1, T0, T0)
	require.NoError(t, err)
	require.True(t, ok)
	require.Empty(t, due(t, s, T0.Add(time.Hour)))

	require.NoError(t, s.Reschedule(t.Context(), "k9", 1, T0))
	got := read(t, s, "k9")
	require.Equal(t, retryqueue.StateExpired, got.State)
	require.Equal(t, T0, got.ExpiredAt)
	require.Zero(t, got.Attempts)

	ok, err = s.Expire(t.Context(), "k9", 1, T0, T0.Add(time.Hour))
	require.NoError(t, err)
	require.False(t, ok, "an expired item expires once")
}

// DeleteIsIdempotent asserts Delete removes the item and a repeat is no error.
func DeleteIsIdempotent(t *testing.T, s Storage) {
	mark(t, s, Pending("k5", T0))
	for range 2 {
		require.NoError(t, s.Delete(t.Context(), "k5"))
	}
	require.Empty(t, due(t, s, T0))
	got, err := s.ListByState(t.Context(), retryqueue.StatePending, 0)
	require.NoError(t, err)
	require.Empty(t, got)
}

// ResolveKeepsPinnedPayload asserts a resolved item is no longer due but a
// repeated Mark (a late duplicate of the work) gets its pinned payload.
//
//nolint:mnd // Fixed instants, generations and counts describe the storage contract.
func ResolveKeepsPinnedPayload(t *testing.T, s Storage) {
	first, err := s.Mark(t.Context(), Pending("k7", T0))
	require.NoError(t, err)
	require.NoError(t, s.Resolve(t.Context(), "k7", 1, T0))
	require.Empty(t, due(t, s, T0.Add(time.Hour)))

	again := Pending("k7", T0)
	again.Payload.ValueOn = T0.Add(30 * time.Hour)
	got, err := s.Mark(t.Context(), again)
	require.NoError(t, err)
	require.Equal(t, retryqueue.StateResolved, got.State)
	require.Equal(t, first.Payload, got.Payload)
	require.Equal(t, T0, got.ResolvedAt)
}

// ResolveIsIdempotentAndClearsExpiry asserts a second Resolve keeps the first
// ResolvedAt and resolving an expired item drops its expiry and flag.
func ResolveIsIdempotentAndClearsExpiry(t *testing.T, s Storage) {
	mark(t, s, Pending("k30", T0), Pending("k31", T0))
	require.NoError(t, s.Resolve(t.Context(), "k30", 1, T0))
	require.NoError(t, s.Resolve(t.Context(), "k30", 1, T0.Add(time.Hour)))

	flagged, err := s.Flag(t.Context(), "k31", 1, "why")
	require.NoError(t, err)
	require.True(t, flagged)
	ok, err := s.Expire(t.Context(), "k31", 1, T0, T0)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, s.Resolve(t.Context(), "k31", 1, T0.Add(time.Hour)))

	require.Equal(t, T0, read(t, s, "k30").ResolvedAt)
	got := read(t, s, "k31")
	require.Equal(t, retryqueue.StateResolved, got.State)
	require.Equal(t, T0.Add(time.Hour), got.ResolvedAt)
	require.Zero(t, got.ExpiredAt)
	require.False(t, got.Flagged)
	require.Empty(t, got.FlagReason)
}

// WakeDueMakesPendingDue asserts WakeDue makes rescheduled pending items due at
// once, leaves resolved and expired ones and already due ones alone, and counts
// no attempt.
func WakeDueMakesPendingDue(t *testing.T, s Storage) {
	mark(t, s, Pending("w1", T0.Add(time.Hour)), Pending("w2", T0.Add(time.Hour)),
		Pending("w3", T0.Add(time.Hour)), Pending("w4", T0.Add(-time.Hour)))
	require.NoError(t, s.Resolve(t.Context(), "w2", 1, T0))
	_, err := s.Expire(t.Context(), "w3", 1, T0, T0)
	require.NoError(t, err)
	require.NoError(t, s.WakeDue(t.Context(), T0))

	got := due(t, s, T0)
	require.Equal(t, []string{"w4", "w1"}, keys(got))
	require.Equal(t, T0.Add(-time.Hour), got[0].NextAttemptAt, "an item already due keeps its schedule")
	require.Equal(t, T0, got[1].NextAttemptAt)
	require.Zero(t, got[1].Attempts)
	require.Zero(t, read(t, s, "w2").NextAttemptAt.Compare(T0.Add(time.Hour)))
}

// FlagOnce asserts Flag reports a change only the first time, so an alert
// fires once.
func FlagOnce(t *testing.T, s Storage) {
	mark(t, s, Pending("f1", T0))
	for i, want := range []bool{true, false} {
		got, err := s.Flag(t.Context(), "f1", 1, "EUR/RUB on 2026-10-17")
		require.NoError(t, err)
		require.Equal(t, want, got, "flag %d", i)
	}
	got := read(t, s, "f1")
	require.True(t, got.Flagged)
	require.Equal(t, "EUR/RUB on 2026-10-17", got.FlagReason)
}

// FlagReasonChangeAndClear asserts a new reason flags again and ClearFlag drops
// the flag.
func FlagReasonChangeAndClear(t *testing.T, s Storage) {
	mark(t, s, Pending("r1", T0))
	for _, tc := range []struct {
		reason string
		want   bool
	}{{"EUR/RUB", true}, {"EUR/RUB", false}, {"USD/RUB", true}} {
		got, err := s.Flag(t.Context(), "r1", 1, tc.reason)
		require.NoError(t, err)
		require.Equal(t, tc.want, got, "flag %s", tc.reason)
	}
	require.Equal(t, "USD/RUB", read(t, s, "r1").FlagReason)
	require.NoError(t, s.ClearFlag(t.Context(), "r1", 1))
	require.NoError(t, s.ClearFlag(t.Context(), "r1", 1))
	got := read(t, s, "r1")
	require.False(t, got.Flagged)
	require.Empty(t, got.FlagReason)
}

// FlagOnlyPending asserts an expired or resolved item is never flagged, while
// ClearFlag applies in any state.
func FlagOnlyPending(t *testing.T, s Storage) {
	mark(t, s, Pending("e", T0), Pending("r", T0), Pending("c", T0))
	_, err := s.Expire(t.Context(), "e", 1, T0, T0)
	require.NoError(t, err)
	require.NoError(t, s.Resolve(t.Context(), "r", 1, T0))
	for _, key := range []string{"e", "r"} {
		got, flagErr := s.Flag(t.Context(), key, 1, "why")
		require.NoError(t, flagErr)
		require.False(t, got, "flag %s", key)
		require.False(t, read(t, s, key).Flagged)
	}

	flagged, err := s.Flag(t.Context(), "c", 1, "why")
	require.NoError(t, err)
	require.True(t, flagged)
	ok, err := s.Expire(t.Context(), "c", 1, T0, T0)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, read(t, s, "c").Flagged, "expiry keeps the flag")
	require.NoError(t, s.ClearFlag(t.Context(), "c", 1))
	require.False(t, read(t, s, "c").Flagged)
}

// RequeueRevivesExpired asserts Requeue moves only expired items back to
// pending with a fresh age, no attempts and no expiry, by key or all, keeping
// generation and flag.
//
//nolint:mnd // Fixed instants, generations and counts describe the storage contract.
func RequeueRevivesExpired(t *testing.T, s Storage) {
	mark(t, s, Pending("e1", T0), Pending("e2", T0), Pending("p1", T0), Pending("r1", T0))
	for _, key := range []string{"e1", "e2"} {
		require.NoError(t, s.Reschedule(t.Context(), key, 1, T0))
		_, err := s.Flag(t.Context(), key, 1, "why")
		require.NoError(t, err)
		ok, err := s.Expire(t.Context(), key, 1, T0, T0)
		require.NoError(t, err)
		require.True(t, ok)
	}
	require.NoError(t, s.Resolve(t.Context(), "r1", 1, T0))
	expired, err := s.ListByState(t.Context(), retryqueue.StateExpired, 10)
	require.NoError(t, err)
	require.Equal(t, []string{"e1", "e2"}, keys(expired))

	later := T0.Add(24 * time.Hour)
	n, err := s.Requeue(t.Context(), later, "e1", "p1", "r1", "missing")
	require.NoError(t, err)
	require.Equal(t, 1, n)
	n, err = s.Requeue(t.Context(), later)
	require.NoError(t, err)
	require.Equal(t, 1, n, "only e2 was left expired")
	n, err = s.Requeue(t.Context(), later)
	require.NoError(t, err)
	require.Zero(t, n)

	got := due(t, s, later)
	require.Equal(t, []string{"p1", "e1", "e2"}, keys(got))
	for _, it := range got[1:] {
		require.Equal(t, later, it.Since, it.Key)
		require.Equal(t, later, it.NextAttemptAt, it.Key)
		require.Zero(t, it.Attempts, it.Key)
		require.Zero(t, it.ExpiredAt, it.Key)
		require.Equal(t, int64(1), it.Generation, it.Key)
		require.True(t, it.Flagged, it.Key)
	}
	require.Equal(t, retryqueue.StateResolved, read(t, s, "r1").State)
}

// ReopenMovesToNewerGenerationOnly asserts Reopen moves a closed item to a
// newer generation with a clean pending state, a stale Reopen returns the newer
// item unchanged, and transitions made with the old generation leave the
// reopened item untouched.
//
//nolint:mnd // Fixed instants, generations and counts describe the storage contract.
func ReopenMovesToNewerGenerationOnly(t *testing.T, s Storage) {
	first, err := s.Mark(t.Context(), Pending("g1", T0))
	require.NoError(t, err)
	_, err = s.Flag(t.Context(), "g1", first.Generation, "EUR/RUB")
	require.NoError(t, err)
	require.NoError(t, s.Reschedule(t.Context(), "g1", first.Generation, T0))
	require.NoError(t, s.Resolve(t.Context(), "g1", first.Generation, T0))

	newer := Payload{ValueOn: T0.Add(time.Hour), Note: "second"}
	reopened, err := s.Reopen(t.Context(), Item{Key: "g1", Since: T0.Add(time.Minute), NextAttemptAt: T0, Generation: 2, Payload: newer})
	require.NoError(t, err)
	require.Equal(t, Item{
		Key: "g1", Generation: 2, State: retryqueue.StatePending, Since: T0.Add(time.Minute), NextAttemptAt: T0, Payload: newer,
	}, reopened)

	stale, err := s.Reopen(t.Context(), Item{Key: "g1", Since: T0, NextAttemptAt: T0, Generation: 1, Payload: Payload{Note: "stale"}})
	require.NoError(t, err)
	require.Equal(t, reopened, stale)

	// Stale attempts of the first generation must not close the new one.
	require.NoError(t, s.Resolve(t.Context(), "g1", first.Generation, T0))
	ok, err := s.Expire(t.Context(), "g1", first.Generation, T0.Add(time.Minute), T0)
	require.NoError(t, err)
	require.False(t, ok)
	newerExists, err := s.DeleteObsolete(t.Context(), "g1", first.Generation)
	require.NoError(t, err)
	require.True(t, newerExists)
	got := due(t, s, T0)
	require.Len(t, got, 1)
	require.Equal(t, reopened, got[0])

	// The current generation may drop it.
	newerExists, err = s.DeleteObsolete(t.Context(), "g1", 2)
	require.NoError(t, err)
	require.False(t, newerExists)
	require.Empty(t, due(t, s, T0))
}

// ReopenSameGeneration asserts Reopen reopens an expired or resolved item of
// the same generation, and returns a pending one of that generation unchanged.
func ReopenSameGeneration(t *testing.T, s Storage) {
	mark(t, s, Pending("p", T0), Pending("e", T0), Pending("r", T0))
	require.NoError(t, s.Reschedule(t.Context(), "p", 1, T0.Add(time.Hour)))
	_, err := s.Expire(t.Context(), "e", 1, T0, T0)
	require.NoError(t, err)
	require.NoError(t, s.Resolve(t.Context(), "r", 1, T0))

	in := Item{Generation: 1, Since: T0.Add(time.Hour), NextAttemptAt: T0.Add(time.Hour), Payload: Payload{Note: "reopened"}}
	pendingBefore := read(t, s, "p")
	in.Key = "p"
	got, err := s.Reopen(t.Context(), in)
	require.NoError(t, err)
	require.Equal(t, pendingBefore, got)

	for _, key := range []string{"e", "r"} {
		in.Key = key
		got, err = s.Reopen(t.Context(), in)
		require.NoError(t, err)
		require.Equal(t, Item{
			Key: key, Generation: 1, State: retryqueue.StatePending, Since: in.Since, NextAttemptAt: in.NextAttemptAt, Payload: in.Payload,
		}, got)
	}
}

// ReopenMissingInserts asserts Reopen of a missing key inserts it as Mark does.
//
//nolint:mnd // Fixed instants, generations and counts describe the storage contract.
func ReopenMissingInserts(t *testing.T, s Storage) {
	in := Pending("new", T0)
	in.Generation = 3
	in.Attempts = 2
	got, err := s.Reopen(t.Context(), in)
	require.NoError(t, err)
	require.Equal(t, Item{
		Key: "new", Generation: 3, State: retryqueue.StatePending, Since: T0, NextAttemptAt: T0, Payload: in.Payload,
	}, got)
	require.Equal(t, got, read(t, s, "new"))
}

// ReopenReplacesPayload asserts Reopen replaces the whole payload: a field the
// new payload leaves empty (omitted from its encoding) does not keep the value
// of the previous generation, and the flag of the previous generation is gone.
//
//nolint:mnd // Fixed instants, generations and counts describe the storage contract.
func ReopenReplacesPayload(t *testing.T, s Storage) {
	first := Pending("rp", T0)
	first.Payload.Tag = "old"
	_, err := s.Mark(t.Context(), first)
	require.NoError(t, err)
	flagged, err := s.Flag(t.Context(), "rp", 1, "why")
	require.NoError(t, err)
	require.True(t, flagged)

	next := Item{Key: "rp", Generation: 2, Since: T0, NextAttemptAt: T0, Payload: Payload{ValueOn: T0, Note: "second"}}
	got, err := s.Reopen(t.Context(), next)
	require.NoError(t, err)
	require.Equal(t, next.Payload, got.Payload)
	require.False(t, got.Flagged)
	require.Empty(t, got.FlagReason)
	stored := read(t, s, "rp")
	require.Equal(t, next.Payload, stored.Payload)
	require.False(t, stored.Flagged)

	// Flag, read and clear at the new generation keep the payload intact.
	flagged, err = s.Flag(t.Context(), "rp", 2, "again")
	require.NoError(t, err)
	require.True(t, flagged)
	listed := due(t, s, T0)
	require.Len(t, listed, 1)
	require.Equal(t, next.Payload, listed[0].Payload)
	require.True(t, listed[0].Flagged)
	require.NoError(t, s.ClearFlag(t.Context(), "rp", 2))
	require.Equal(t, next.Payload, read(t, s, "rp").Payload)
}

// StaleExpireSparesRequeued asserts an attempt that read the item before an
// operator requeued it cannot expire the requeued item.
func StaleExpireSparesRequeued(t *testing.T, s Storage) {
	mark(t, s, Pending("q1", T0))
	_, err := s.Expire(t.Context(), "q1", 1, T0, T0)
	require.NoError(t, err)
	later := T0.Add(time.Hour)
	n, err := s.Requeue(t.Context(), later, "q1")
	require.NoError(t, err)
	require.Equal(t, 1, n)

	ok, err := s.Expire(t.Context(), "q1", 1, T0, later) // the stale attempt's age
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, []string{"q1"}, keys(due(t, s, later)))

	ok, err = s.Expire(t.Context(), "q1", 1, later, later) // the current age
	require.NoError(t, err)
	require.True(t, ok)
}

// StaleGenerationIsNoOp asserts every generation-fenced transition made with
// another generation leaves the item unchanged.
//
//nolint:mnd // Fixed instants, generations and counts describe the storage contract.
func StaleGenerationIsNoOp(t *testing.T, s Storage) {
	mark(t, s, Pending("s1", T0))
	before := read(t, s, "s1")
	for _, gen := range []int64{0, 2} {
		require.NoError(t, s.Reschedule(t.Context(), "s1", gen, T0.Add(time.Hour)))
		flagged, err := s.Flag(t.Context(), "s1", gen, "why")
		require.NoError(t, err)
		require.False(t, flagged)
		ok, err := s.Expire(t.Context(), "s1", gen, T0, T0)
		require.NoError(t, err)
		require.False(t, ok)
		require.NoError(t, s.Resolve(t.Context(), "s1", gen, T0))
	}
	require.Equal(t, before, read(t, s, "s1"))

	_, err := s.Flag(t.Context(), "s1", 1, "why")
	require.NoError(t, err)
	require.NoError(t, s.ClearFlag(t.Context(), "s1", 2))
	require.True(t, read(t, s, "s1").Flagged, "a stale ClearFlag keeps the flag")
}

// ListByStateOrder asserts ListByState filters by state, orders by age then key
// and honors the limit.
func ListByStateOrder(t *testing.T, s Storage) {
	older := Pending("z", T0)
	older.Since = T0.Add(-time.Hour)
	mark(t, s, Pending("b", T0), Pending("a", T0), older, Pending("x", T0))
	require.NoError(t, s.Resolve(t.Context(), "x", 1, T0))

	got, err := s.ListByState(t.Context(), retryqueue.StatePending, 0)
	require.NoError(t, err)
	require.Equal(t, []string{"z", "a", "b"}, keys(got))
	got, err = s.ListByState(t.Context(), retryqueue.StatePending, 1)
	require.NoError(t, err)
	require.Equal(t, []string{"z"}, keys(got))
	got, err = s.ListByState(t.Context(), retryqueue.StateResolved, 0)
	require.NoError(t, err)
	require.Equal(t, []string{"x"}, keys(got))
	got, err = s.ListByState(t.Context(), retryqueue.StateExpired, 0)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Empty(t, got)
}

// MissingItemIsNoOp asserts every transition on a missing key succeeds without
// creating it.
func MissingItemIsNoOp(t *testing.T, s Storage) {
	ctx := t.Context()
	require.NoError(t, s.Reschedule(ctx, "none", 1, T0))
	ok, err := s.Expire(ctx, "none", 1, T0, T0)
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, s.Resolve(ctx, "none", 1, T0))
	flagged, err := s.Flag(ctx, "none", 1, "why")
	require.NoError(t, err)
	require.False(t, flagged)
	require.NoError(t, s.ClearFlag(ctx, "none", 1))
	newer, err := s.DeleteObsolete(ctx, "none", 1)
	require.NoError(t, err)
	require.False(t, newer)
	require.NoError(t, s.Delete(ctx, "none"))
	require.NoError(t, s.WakeDue(ctx, T0))
	n, err := s.Requeue(ctx, T0, "none")
	require.NoError(t, err)
	require.Zero(t, n)
	for _, state := range []retryqueue.State{retryqueue.StatePending, retryqueue.StateExpired, retryqueue.StateResolved} {
		got, err := s.ListByState(ctx, state, 0)
		require.NoError(t, err)
		require.Empty(t, got)
	}
}

// ConcurrentMark asserts that of concurrent first marks with different
// payloads exactly one inserts and every caller gets that item.
func ConcurrentMark(t *testing.T, s Storage) {
	results := make([]Item, concurrency)
	errs := make([]error, concurrency)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range concurrency {
		wg.Go(func() {
			<-start
			in := Pending("race", T0)
			in.Payload.Note = fmt.Sprintf("writer-%d", i)
			results[i], errs[i] = s.Mark(t.Context(), in)
		})
	}
	close(start)
	wg.Wait()
	for i := range concurrency {
		require.NoError(t, errs[i], "writer %d", i)
		require.Equal(t, results[0], results[i], "writer %d", i)
	}
	require.Equal(t, results[0], read(t, s, "race"))
}

// ConcurrentExpire asserts that of concurrent expiries of one item exactly one
// reports the transition.
func ConcurrentExpire(t *testing.T, s Storage) {
	mark(t, s, Pending("race", T0))
	var won atomic.Int32
	raceAll(t, func() error {
		ok, err := s.Expire(t.Context(), "race", 1, T0, T0)
		if ok {
			won.Add(1)
		}
		return err
	})
	require.Equal(t, int32(1), won.Load())
	require.Equal(t, retryqueue.StateExpired, read(t, s, "race").State)
}

// ConcurrentFlag asserts that of concurrent flags with one reason exactly one
// reports a change, so the alert fires once.
func ConcurrentFlag(t *testing.T, s Storage) {
	mark(t, s, Pending("race", T0))
	var won atomic.Int32
	raceAll(t, func() error {
		ok, err := s.Flag(t.Context(), "race", 1, "why")
		if ok {
			won.Add(1)
		}
		return err
	})
	require.Equal(t, int32(1), won.Load())
}

// raceAll runs fn from concurrency goroutines released together and fails the
// test on any error.
func raceAll(t *testing.T, fn func() error) {
	t.Helper()
	errs := make([]error, concurrency)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range concurrency {
		wg.Go(func() {
			<-start
			errs[i] = fn()
		})
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "goroutine %d", i)
	}
}

// mover returns s as a [retryqueue.Mover], skipping the test when s is not one.
func mover(t *testing.T, s Storage) retryqueue.Mover {
	t.Helper()
	m, ok := s.(retryqueue.Mover)
	if !ok {
		t.Skip("the store does not implement retryqueue.Mover")
	}
	return m
}

// all returns every stored item, in any state, ordered by key.
//
//nolint:mnd // Fixed instants, generations and counts describe the storage contract.
func all(t *testing.T, s Storage) []Item {
	t.Helper()
	var out []Item
	for _, state := range []retryqueue.State{retryqueue.StatePending, retryqueue.StateExpired, retryqueue.StateResolved} {
		got, err := s.ListByState(t.Context(), state, 100)
		require.NoError(t, err)
		out = append(out, got...)
	}
	slices.SortFunc(out, func(a, b Item) int { return strings.Compare(a.Key, b.Key) })
	return out
}

// MoveKeepsEveryField asserts Move rekeys an item in each state with every
// other field as stored: generation, state, age, attempts, schedule, expiry,
// resolution, flag and payload.
//
//nolint:mnd // Fixed instants, generations and counts describe the storage contract.
func MoveKeepsEveryField(t *testing.T, s Storage) {
	m := mover(t, s)
	ctx := t.Context()

	// A pending item at generation 3 with attempts, a schedule, a flag and a
	// payload carrying the omitempty field.
	in := Pending("pending", T0.Add(time.Minute))
	in.Generation = 3
	in.Payload = Payload{ValueOn: T0.Add(-2 * time.Hour), Note: "pinned", Tag: "kept"}
	mark(t, s, in)
	require.NoError(t, s.Reschedule(ctx, "pending", 3, T0.Add(time.Hour)))
	require.NoError(t, s.Reschedule(ctx, "pending", 3, T0.Add(2*time.Hour)))
	flagged, err := s.Flag(ctx, "pending", 3, "why")
	require.NoError(t, err)
	require.True(t, flagged)

	// An expired item, flagged before it expired, and a resolved one.
	mark(t, s, Pending("expired", T0), Pending("resolved", T0))
	flagged, err = s.Flag(ctx, "expired", 1, "stuck")
	require.NoError(t, err)
	require.True(t, flagged)
	expired, err := s.Expire(ctx, "expired", 1, T0, T0.Add(3*time.Hour))
	require.NoError(t, err)
	require.True(t, expired)
	require.NoError(t, s.Resolve(ctx, "resolved", 1, T0.Add(4*time.Hour)))

	before := all(t, s)
	require.Len(t, before, 3)
	for _, it := range before {
		require.NoError(t, m.Move(ctx, it.Key, "moved-"+it.Key))
	}

	after := all(t, s)
	require.Len(t, after, 3, "every item is under its new key only")
	for i, it := range before {
		want := it
		want.Key = "moved-" + it.Key
		require.Equal(t, want, after[i], "item %s", it.Key)
	}
	got := read(t, s, "moved-pending")
	require.Equal(t, 2, got.Attempts)
	require.Equal(t, int64(3), got.Generation)
	require.True(t, got.Flagged)
	require.Equal(t, "why", got.FlagReason)
	require.Equal(t, "kept", got.Payload.Tag)

	// The moved item stays under the fences it had: its generation and age.
	ok, err := s.Expire(ctx, "moved-pending", 3, T0, T0.Add(5*time.Hour))
	require.NoError(t, err)
	require.True(t, ok)
}

// MoveRefusals asserts Move fails with [retryqueue.ErrItemNotFound] from a key
// without an item and with [retryqueue.ErrItemExists] onto a taken key, and
// changes nothing either way.
func MoveRefusals(t *testing.T, s Storage) {
	m := mover(t, s)
	ctx := t.Context()

	require.ErrorIs(t, m.Move(ctx, "missing", "free"), retryqueue.ErrItemNotFound)
	require.Empty(t, all(t, s))

	first := Pending("a", T0)
	second := Pending("b", T0.Add(time.Minute))
	second.Payload.Note = "second"
	mark(t, s, first, second)
	before := all(t, s)

	require.ErrorIs(t, m.Move(ctx, "a", "b"), retryqueue.ErrItemExists)
	require.ErrorIs(t, m.Move(ctx, "missing", "b"), retryqueue.ErrItemNotFound, "a missing source wins over a taken target")
	require.Equal(t, before, all(t, s))
}

// MoveOntoItself asserts a Move onto the item's own key changes nothing and
// still reports a missing item.
func MoveOntoItself(t *testing.T, s Storage) {
	m := mover(t, s)
	ctx := t.Context()

	require.ErrorIs(t, m.Move(ctx, "self", "self"), retryqueue.ErrItemNotFound)
	mark(t, s, Pending("self", T0))
	before := all(t, s)
	require.NoError(t, m.Move(ctx, "self", "self"))
	require.Equal(t, before, all(t, s))
}

// ConcurrentMoveFromOneKey asserts that of concurrent moves of one item to
// different keys exactly one succeeds and the others find no item.
func ConcurrentMoveFromOneKey(t *testing.T, s Storage) {
	m := mover(t, s)
	mark(t, s, Pending("race", T0))
	var won, missed atomic.Int32
	var next atomic.Int32
	raceAll(t, func() error {
		err := m.Move(t.Context(), "race", fmt.Sprintf("to-%d", next.Add(1)))
		switch {
		case err == nil:
			won.Add(1)
		case errors.Is(err, retryqueue.ErrItemNotFound):
			missed.Add(1)
		default:
			return err
		}
		return nil
	})
	require.Equal(t, int32(1), won.Load())
	require.Equal(t, int32(concurrency-1), missed.Load())
	got := all(t, s)
	require.Len(t, got, 1)
	require.NotEqual(t, "race", got[0].Key)
}

// ConcurrentMoveOntoOneKey asserts that of concurrent moves of different items
// onto one key exactly one succeeds, the others find the key taken, and every
// item is kept.
func ConcurrentMoveOntoOneKey(t *testing.T, s Storage) {
	m := mover(t, s)
	for i := range concurrency {
		mark(t, s, Pending(fmt.Sprintf("from-%02d", i), T0))
	}
	var won, taken atomic.Int32
	var next atomic.Int32
	raceAll(t, func() error {
		err := m.Move(t.Context(), fmt.Sprintf("from-%02d", next.Add(1)-1), "target")
		switch {
		case err == nil:
			won.Add(1)
		case errors.Is(err, retryqueue.ErrItemExists):
			taken.Add(1)
		default:
			return err
		}
		return nil
	})
	require.Equal(t, int32(1), won.Load())
	require.Equal(t, int32(concurrency-1), taken.Load())
	got := all(t, s)
	require.Len(t, got, concurrency)
	require.True(t, slices.ContainsFunc(got, func(it Item) bool { return it.Key == "target" }))
}

// DoneContextChangesNothing asserts every method called with a canceled
// context fails with an error that wraps [context.Canceled] and leaves the
// store as it was (Move too, for a [retryqueue.Mover]).
//
//nolint:mnd // Fixed instants, generations and counts describe the storage contract.
func DoneContextChangesNothing(t *testing.T, s Storage) {
	mark(t, s, Pending("pending", T0), Pending("expired", T0))
	expired, err := s.Expire(t.Context(), "expired", 1, T0, T0)
	require.NoError(t, err)
	require.True(t, expired)
	before := all(t, s)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	later := T0.Add(time.Hour)
	calls := map[string]func() error{
		"Mark": func() error { _, err := s.Mark(ctx, Pending("new", T0)); return err },
		"Reopen": func() error {
			in := Pending("pending", T0)
			in.Generation = 2
			_, err := s.Reopen(ctx, in)
			return err
		},
		"ListDue":        func() error { _, err := s.ListDue(ctx, later, 0); return err },
		"WakeDue":        func() error { return s.WakeDue(ctx, T0.Add(-time.Hour)) },
		"Reschedule":     func() error { return s.Reschedule(ctx, "pending", 1, later) },
		"Expire":         func() error { _, err := s.Expire(ctx, "pending", 1, T0, later); return err },
		"Resolve":        func() error { return s.Resolve(ctx, "pending", 1, later) },
		"Flag":           func() error { _, err := s.Flag(ctx, "pending", 1, "why"); return err },
		"ClearFlag":      func() error { return s.ClearFlag(ctx, "pending", 1) },
		"DeleteObsolete": func() error { _, err := s.DeleteObsolete(ctx, "pending", 1); return err },
		"Delete":         func() error { return s.Delete(ctx, "pending") },
		"ListByState":    func() error { _, err := s.ListByState(ctx, retryqueue.StatePending, 0); return err },
		"Requeue":        func() error { _, err := s.Requeue(ctx, later); return err },
	}
	if m, ok := s.(retryqueue.Mover); ok {
		calls["Move"] = func() error { return m.Move(ctx, "pending", "moved") }
	}
	for name, call := range calls {
		require.ErrorIs(t, call(), context.Canceled, name)
	}
	require.Equal(t, before, all(t, s))
}
