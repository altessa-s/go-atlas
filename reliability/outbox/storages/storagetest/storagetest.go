// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//nolint:mnd // Fixture sizes, batch sizes and windows are fixed values of the contracts.
package storagetest

import (
	"context"
	"crypto/rand"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/outbox"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// contracts lists the storage contracts; [Run] executes each.
var contracts = []struct {
	name  string
	check func(*testing.T, outbox.Storage)
}{
	{"FetchOldestFirst", FetchOldestFirst},
	{"FetchBatchSize", FetchBatchSize},
	{"FetchZeroBatch", FetchZeroBatch},
	{"FetchInterleavesFailed", FetchInterleavesFailed},
	{"PayloadsRoundTrip", PayloadsRoundTrip},
	{"InstantsRoundTrip", InstantsRoundTrip},
	{"ConcurrentFetch", ConcurrentFetch},
	{"UpdateFencing", UpdateFencing},
	{"UnlockKeepsLiveLeases", UnlockKeepsLiveLeases},
	{"RetryBackoff", RetryBackoff},
	{"Expire", Expire},
	{"DeleteKeepsDeadLetters", DeleteKeepsDeadLetters},
	{"Stats", Stats},
}

// Run executes every storage contract as a parallel subtest named after it,
// each on a fresh store from newStore, which must return an empty store
// isolated from every other store it returns. The contracts named in skip are
// reported as skipped.
func Run(t *testing.T, newStore func(testing.TB) outbox.Storage, skip ...string) {
	t.Helper()
	run(t, newStore, nil, skip)
}

// RunWithOrderKeys runs the same suite and pins the backend's documented
// ordering-key format. Pass true for an effective ordering key and false
// for a topic-only store.
func RunWithOrderKeys(t *testing.T, newStore func(testing.TB) outbox.Storage, persists bool, skip ...string) {
	t.Helper()
	run(t, newStore, &persists, skip)
}

func run(t *testing.T, newStore func(testing.TB) outbox.Storage, persists *bool, skip []string) {
	t.Helper()
	t.Run("OrderKeys", func(t *testing.T) {
		t.Parallel()
		if slices.Contains(skip, "OrderKeys") {
			t.Skip("not supported by this backend")
		}
		hasOrderKey := checkOrderKeys(t, newStore(t))
		if persists != nil {
			require.Equal(t, *persists, hasOrderKey, "backend ordering-key format")
		}
	})
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

// Window is how long the contracts wait for an asynchronous outcome — a
// notification in [Watch].
const Window = 10 * time.Second

// concurrency is the number of goroutines racing in [ConcurrentFetch].
const concurrency = 8

// settle is slept before a lock-age predicate with a zero threshold, so the
// backend clock has moved past the instant the lock was stamped at whatever
// precision the backend keeps (milliseconds on MongoDB).
const settle = 20 * time.Millisecond

// eventSeq disambiguates event IDs within one process.
var eventSeq atomic.Int64

// newID returns an event ID unique across processes sharing a backend.
func newID() string {
	return fmt.Sprintf("st-%s-%d", rand.Text()[:8], eventSeq.Add(1))
}

// pending builds a pending event created at the given instant, truncated to
// milliseconds — the precision every backend keeps.
func pending(key string, created time.Time) outbox.Event {
	return outbox.Event{
		Id:        newID(),
		Key:       key,
		Payload:   []byte(key),
		Status:    outbox.StatusPending,
		CreatedAt: created.UTC().Truncate(time.Millisecond),
	}
}

// saveAscending saves n pending events created a millisecond apart, oldest
// first, and returns them in that order.
func saveAscending(t *testing.T, store outbox.Storage, n int) []outbox.Event {
	t.Helper()
	base := time.Now().Add(-time.Minute)
	events := make([]outbox.Event, n)
	for i := range events {
		events[i] = pending(fmt.Sprintf("k.%d", i), base.Add(time.Duration(i)*time.Millisecond))
	}
	require.NoError(t, store.SaveEvents(t.Context(), events...))
	return events
}

// fetch fetches up to n ready events and requires the call to succeed.
func fetch(t *testing.T, store outbox.Storage, n uint32) []outbox.Event {
	t.Helper()
	events, err := store.FetchUnprocessedEvents(t.Context(), n)
	require.NoError(t, err)
	return events
}

// ids returns the IDs of events, in order.
func ids(events []outbox.Event) []string {
	out := make([]string, len(events))
	for i, ev := range events {
		out[i] = ev.Id
	}
	return out
}

// FetchOldestFirst checks that a fetch returns ready events oldest first, each
// locked with a token, and that locked events are not returned again.
func FetchOldestFirst(t *testing.T, store outbox.Storage) {
	saved := saveAscending(t, store, 3)
	// Saved out of order: the fetch, not the insert, decides the order.
	late := pending("k.late", saved[0].CreatedAt.Add(-time.Second))
	require.NoError(t, store.SaveEvents(t.Context(), late))

	got := fetch(t, store, 10)
	require.Equal(t, append([]string{late.Id}, ids(saved)...), ids(got))
	for _, ev := range got {
		require.NotEmpty(t, ev.LockToken, "a fetched event carries its lock token")
	}
	require.Empty(t, fetch(t, store, 10), "locked events are not fetched again")
}

// FetchBatchSize checks that a fetch takes at most the batch size, oldest first.
func FetchBatchSize(t *testing.T, store outbox.Storage) {
	saved := saveAscending(t, store, 5)

	require.Equal(t, ids(saved[:2]), ids(fetch(t, store, 2)))
	require.Equal(t, ids(saved[2:]), ids(fetch(t, store, 10)))
}

// FetchInterleavesFailed checks that a failed event ready for retry is fetched
// in creation order among pending events: before newer pending events, not
// after them.
func FetchInterleavesFailed(t *testing.T, store outbox.Storage) {
	saved := saveAscending(t, store, 3)
	oldest := fetch(t, store, 1)
	require.Equal(t, ids(saved[:1]), ids(oldest))

	msg := "destination unavailable"
	oldest[0].Status, oldest[0].Attempts, oldest[0].LastError = outbox.StatusFailed, 1, &msg
	require.NoError(t, store.UpdateEvents(t.Context(), oldest...))

	require.Equal(t, ids(saved[:2]), ids(fetch(t, store, 2)), "the failed event comes first, then the oldest pending one")
	require.Equal(t, ids(saved[2:]), ids(fetch(t, store, 10)))
}

// PayloadsRoundTrip checks that payload nil-ness and bytes survive the store:
// nil stays nil, empty stays empty, and arbitrary bytes come back unchanged.
func PayloadsRoundTrip(t *testing.T, store outbox.Storage) {
	created := time.Now().Add(-time.Minute)
	events := []outbox.Event{pending("k.nil", created), pending("k.empty", created), pending("k.bytes", created)}
	events[0].Payload = nil
	events[1].Payload = []byte{}
	events[2].Payload = []byte{0, 1, 0xff, '"', '\\'}
	require.NoError(t, store.SaveEvents(t.Context(), events...))

	got := map[string][]byte{}
	for _, ev := range fetch(t, store, 10) {
		got[ev.Id] = ev.Payload
	}
	require.Len(t, got, 3)
	require.Nil(t, got[events[0].Id])
	require.NotNil(t, got[events[1].Id])
	require.Empty(t, got[events[1].Id])
	require.Equal(t, events[2].Payload, got[events[2].Id])
}

// InstantsRoundTrip checks that caller-supplied CreatedAt and ExpiresAt come
// back exactly, at millisecond precision, and that the key survives.
func InstantsRoundTrip(t *testing.T, store outbox.Storage) {
	ev := pending("orders.created ü", time.Now().Add(-time.Minute))
	ev.ExpiresAt = ev.CreatedAt.Add(time.Hour)
	require.NoError(t, store.SaveEvents(t.Context(), ev))

	got := fetch(t, store, 10)
	require.Len(t, got, 1, "an event expiring in an hour is due")
	require.Equal(t, ev.Id, got[0].Id)
	require.Equal(t, ev.Key, got[0].Key)
	require.True(t, ev.CreatedAt.Equal(got[0].CreatedAt), "CreatedAt %v != %v", ev.CreatedAt, got[0].CreatedAt)
	require.True(t, ev.ExpiresAt.Equal(got[0].ExpiresAt), "ExpiresAt %v != %v", ev.ExpiresAt, got[0].ExpiresAt)
}

// ConcurrentFetch checks that concurrent fetches take disjoint batches: every
// event is handed out exactly once.
func ConcurrentFetch(t *testing.T, store outbox.Storage) {
	saved := saveAscending(t, store, 120)

	const batchSize = 7
	var claimed []string
	for len(claimed) < len(saved) {
		var wg sync.WaitGroup
		var batches [concurrency][]outbox.Event
		var errs [concurrency]error
		for i := range batches {
			wg.Go(func() {
				batches[i], errs[i] = store.FetchUnprocessedEvents(t.Context(), batchSize)
			})
		}
		wg.Wait()
		before := len(claimed)
		for i, events := range batches {
			require.NoError(t, errs[i])
			claimed = append(claimed, ids(events)...)
		}
		if len(claimed) == before {
			// SKIP LOCKED can return empty while competing scans hold locks.
			// After the wave finishes, a lone fetch must make progress.
			events := fetch(t, store, batchSize)
			require.NotEmpty(t, events, "saved events remain unclaimed")
			claimed = append(claimed, ids(events)...)
		}
	}
	require.ElementsMatch(t, ids(saved), claimed)
	require.Empty(t, fetch(t, store, batchSize))
}

// UpdateFencing checks the lock-token fence: once the unlock sweep reclaimed an
// event and another fetch locked it again, a write carrying the old token is
// dropped, and one carrying the current token applies.
func UpdateFencing(t *testing.T, store outbox.Storage) {
	saveAscending(t, store, 1)

	first := fetch(t, store, 1)
	require.Len(t, first, 1)
	time.Sleep(settle)
	require.NoError(t, store.UnlockStuckEvents(t.Context(), 0))
	second := fetch(t, store, 1)
	require.Len(t, second, 1)
	require.NotEqual(t, first[0].LockToken, second[0].LockToken, "a new lock gets a new token")

	stale := first[0]
	stale.Status, stale.Attempts, stale.PublishedAt = outbox.StatusSent, 1, time.Now()
	require.NoError(t, store.UpdateEvents(t.Context(), stale))

	// The stale write was dropped: the event is still locked by the second
	// fetch, so reclaiming it makes it ready again.
	time.Sleep(settle)
	require.NoError(t, store.UnlockStuckEvents(t.Context(), 0))
	third := fetch(t, store, 1)
	require.Len(t, third, 1, "a write with a revoked token must not apply")

	current := third[0]
	current.Status, current.Attempts, current.PublishedAt = outbox.StatusSent, 1, time.Now()
	require.NoError(t, store.UpdateEvents(t.Context(), current))
	time.Sleep(settle)
	require.NoError(t, store.UnlockStuckEvents(t.Context(), 0))
	require.Empty(t, fetch(t, store, 1), "a write with the current token applies")
}

// UnlockKeepsLiveLeases checks that the unlock sweep leaves a lock younger
// than the expiry alone.
func UnlockKeepsLiveLeases(t *testing.T, store outbox.Storage) {
	saveAscending(t, store, 1)
	require.Len(t, fetch(t, store, 1), 1)

	require.NoError(t, store.UnlockStuckEvents(t.Context(), time.Hour))
	require.Empty(t, fetch(t, store, 1))
}

// RetryBackoff checks that a failed event is withheld until its RetryAfter
// elapses, and that one without a backoff is ready again at once.
func RetryBackoff(t *testing.T, store outbox.Storage) {
	saveAscending(t, store, 2)
	locked := fetch(t, store, 2)
	require.Len(t, locked, 2)

	msg := "destination unavailable"
	for i := range locked {
		locked[i].Status, locked[i].Attempts, locked[i].LastError = outbox.StatusFailed, 1, &msg
	}
	locked[0].RetryAfter = time.Hour
	require.NoError(t, store.UpdateEvents(t.Context(), locked...))

	got := fetch(t, store, 10)
	require.Equal(t, []string{locked[1].Id}, ids(got), "only the event without a backoff is ready")
	require.Equal(t, uint32(1), got[0].Attempts)
	require.NotNil(t, got[0].LastError)
	require.Equal(t, msg, *got[0].LastError)
}

// Expire checks that ExpireEvents marks pending events past their ExpiresAt as
// expired, which takes them out of dispatch, and leaves the others alone.
func Expire(t *testing.T, store outbox.Storage) {
	created := time.Now().Add(-2 * time.Hour)
	expired, live := pending("k.expired", created), pending("k.live", created)
	expired.ExpiresAt = created.Add(time.Hour)
	live.ExpiresAt = created.Add(4 * time.Hour)
	require.NoError(t, store.SaveEvents(t.Context(), expired, live))

	n, err := store.ExpireEvents(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
	require.Equal(t, []string{live.Id}, ids(fetch(t, store, 10)))
}

// DeleteKeepsDeadLetters checks that the retention sweep never removes a
// dead-lettered event.
func DeleteKeepsDeadLetters(t *testing.T, store outbox.Storage) {
	saveAscending(t, store, 3)
	locked := fetch(t, store, 3)
	require.Len(t, locked, 3)

	now := time.Now()
	locked[0].Status, locked[0].PublishedAt = outbox.StatusSent, now
	locked[1].Status, locked[1].Attempts = outbox.StatusMaxAttemptReached, 1
	locked[2].Status, locked[2].Attempts = outbox.StatusRejected, 1
	require.NoError(t, store.UpdateEvents(t.Context(), locked...))

	time.Sleep(settle)
	require.NoError(t, store.DeleteProcessedEvents(t.Context(), 0))

	stats, err := store.Stats(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(2), stats.DeadLettered)
	require.Zero(t, stats.Pending)
	require.Zero(t, stats.InProgress)
}

// Stats checks the backlog snapshot: pending (failed included), in progress,
// dead-lettered, and the age of the oldest waiting event on the backend clock.
func Stats(t *testing.T, store outbox.Storage) {
	empty, err := store.Stats(t.Context())
	require.NoError(t, err)
	require.Equal(t, outbox.Stats{}, empty, "an empty store reports zeros")

	saveAscending(t, store, 4) // Created about a minute ago.
	locked := fetch(t, store, 2)
	require.Len(t, locked, 2)
	locked[0].Status, locked[0].Attempts = outbox.StatusMaxAttemptReached, 1
	locked = locked[:1]
	require.NoError(t, store.UpdateEvents(t.Context(), locked...))

	stats, err := store.Stats(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(2), stats.Pending)
	require.Equal(t, int64(1), stats.InProgress)
	require.Equal(t, int64(1), stats.DeadLettered)
	require.InDelta(t, time.Minute.Seconds(), stats.OldestPendingAge.Seconds(), 30, "the backlog is about a minute old")
}

// Watch checks the [outbox.Watcher] capability: a committed save makes Watch
// call notify within [Window], and Watch returns nil once its context ends.
// store must implement outbox.Watcher and be able to serve it. Since a
// watcher may subscribe asynchronously, the contract keeps saving until the
// first notification arrives.
func Watch(t *testing.T, store outbox.Storage) {
	watcher, ok := store.(outbox.Watcher)
	require.True(t, ok, "the store must implement outbox.Watcher")

	var notified atomic.Int64
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- watcher.Watch(ctx, func() { notified.Add(1) }) }()

	deadline := time.Now().Add(Window)
	for notified.Load() == 0 {
		require.True(t, time.Now().Before(deadline), "no notification within %s", Window)
		require.NoError(t, store.SaveEvents(t.Context(), pending("k.watch", time.Now())))
		select {
		case err := <-done:
			require.FailNow(t, "Watch returned before its context ended", "error: %v", err)
		case <-time.After(200 * time.Millisecond):
		}
	}

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(Window):
		require.FailNow(t, "Watch did not return after its context ended")
	}
}

// FetchZeroBatch checks rejection of a zero batch size without taking any locks.
func FetchZeroBatch(t *testing.T, store outbox.Storage) {
	t.Helper()
	saved := saveAscending(t, store, 1)
	got, err := store.FetchUnprocessedEvents(t.Context(), 0)
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	require.Empty(t, got)
	require.Equal(t, ids(saved), ids(fetch(t, store, 1)))
}

// checkOrderKeys checks the topic and both groups, and reports whether the
// store returned an explicit ordering key.
func checkOrderKeys(t *testing.T, store outbox.Storage) bool {
	t.Helper()
	at := time.Now().Add(-time.Minute)
	explicit, fallback := pending("created", at), pending("updated", at.Add(time.Millisecond))
	explicit.OrderKey = "order-17"
	require.NoError(t, store.SaveEvents(t.Context(), explicit, fallback))
	got := fetch(t, store, 2)
	require.Equal(t, []string{explicit.Id, fallback.Id}, ids(got))
	require.Equal(t, explicit.Key, got[0].Key)
	require.Equal(t, fallback.Key, got[1].Key)
	if got[0].OrderKey == "" {
		require.Empty(t, got[1].OrderKey)
		return false
	}
	require.Equal(t, explicit.OrderKey, got[0].OrderKey)
	require.Equal(t, fallback.Key, got[1].OrderKey)
	return true
}
