// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/outbox"

	"github.com/altessa-s/go-atlas/tests/integration/outboxit"
)

// An event past its deadline is never handed to the handler, and the expire
// cycle records why it was dropped instead of leaving it pending forever.
func TestLifecycle_ExpiredEventsAreNeverDispatched(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ob := f.newOutbox(t)

	past := time.Now().UTC().Add(-time.Hour)
	saved := f.save(t, ob, outbox.Event{
		Key:       "billing.invoice.paid",
		Payload:   []byte(`{"invoice":1}`),
		ExpiresAt: past,
	})

	require.NoError(t, ob.RunDispatchCycle(t.Context()))
	require.Zero(t, f.recorder.Count(), "an expired event must not be published:\n%s", f.recorder.Timeline())

	require.NoError(t, ob.RunExpireCycle(t.Context()))

	doc := f.load(t, saved[0].Id)
	require.Equal(t, string(outbox.StatusExpired), doc.Status)
	require.NotNil(t, doc.PublishedAt, "expiry must stamp a completion time so cleanup can age it out")
}

// An event whose deadline is still ahead is dispatched normally — the expiry
// filter must not swallow live traffic.
func TestLifecycle_UnexpiredEventsAreDispatchedNormally(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ob := f.newOutbox(t)

	saved := f.save(t, ob, outbox.Event{
		Key:       "billing.invoice.paid",
		Payload:   []byte(`{"invoice":1}`),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})

	require.NoError(t, ob.RunDispatchCycle(t.Context()))
	require.Equal(t, string(outbox.StatusSent), f.load(t, saved[0].Id).Status)
}

// Retention sweeps the events that completed successfully and keeps the ones an
// operator still has to look at. A cleanup that took the dead letters with it
// would turn a visible backlog into silent data loss.
func TestLifecycle_CleanupKeepsDeadLetteredEvents(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	// One event goes through; the other exhausts a budget of one.
	ob := f.newOutbox(t,
		outbox.WithRetryMaxAttempts(1),
		outbox.WithPublishedEventsLifetime(time.Minute),
	)

	delivered := f.save(t, ob, event("billing.invoice.paid", `{"invoice":1}`))
	require.NoError(t, ob.RunDispatchCycle(t.Context()))
	require.Equal(t, string(outbox.StatusSent), f.load(t, delivered[0].Id).Status)

	f.recorder.Respond(func(outboxit.Delivery) error { return errors.New("permanently broken destination") })
	poisoned := f.save(t, ob, event("billing.invoice.failed", `{"invoice":2}`))
	require.NoError(t, ob.RunDispatchCycle(t.Context()))
	require.Equal(t, string(outbox.StatusMaxAttemptReached), f.load(t, poisoned[0].Id).Status)

	// Retention is a duration measured against the server clock on both sides —
	// the deadline and the completion timestamp alike — so the sweep is driven
	// by how old the events are and not by how far the caller's clock has
	// drifted from the database's.
	retention := 50 * time.Millisecond
	time.Sleep(2 * retention)
	require.NoError(t, f.store.DeleteProcessedEvents(t.Context(), retention))

	remaining := f.loadAll(t)
	require.Len(t, remaining, 1, "the sent event must be swept and the dead letter kept")
	require.Equal(t, poisoned[0].Id, remaining[0].ID)
	require.Equal(t, string(outbox.StatusMaxAttemptReached), remaining[0].Status)
}

// Retention leaves events alone until they are actually old enough — a sweep
// must not delete something published a moment ago.
func TestLifecycle_CleanupRespectsTheRetentionWindow(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ob := f.newOutbox(t, outbox.WithPublishedEventsLifetime(time.Hour))

	saved := f.save(t, ob, event("billing.invoice.paid", `{"invoice":1}`))
	require.NoError(t, ob.RunDispatchCycle(t.Context()))

	require.NoError(t, ob.RunCleanupCycle(t.Context()))

	require.Equal(t, int64(1), f.countEvents(t), "a freshly sent event is not yet due for cleanup")
	require.Equal(t, string(outbox.StatusSent), f.load(t, saved[0].Id).Status)
}

// The gauges an operator alerts on. Counters cannot tell a stalled outbox from
// an idle one — both report zero — so these numbers are the only signal that
// something is piling up, and they are computed against the server clock.
func TestLifecycle_StatsReportBacklogDeadLettersAndLag(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ob := f.newOutbox(t, outbox.WithRetryMaxAttempts(1))

	// Dead-letter one event first. A dispatch cycle drains everything that is
	// eligible, so the backlog has to be created after this one is terminal —
	// otherwise it would be dead-lettered too and there would be no backlog
	// left to measure.
	f.recorder.Respond(func(outboxit.Delivery) error { return errors.New("permanently broken destination") })
	poisoned := f.save(t, ob, event("orders.3", "c"))
	require.NoError(t, ob.RunDispatchCycle(t.Context()))
	require.Equal(t, string(outbox.StatusMaxAttemptReached), f.load(t, poisoned[0].Id).Status)

	// Two events that are never dispatched in this test: pure backlog. One is
	// backdated so the lag is unambiguous.
	backdated := time.Now().UTC().Add(-2 * time.Hour)
	f.save(t, ob,
		outbox.Event{Key: "orders.1", Payload: []byte("a"), CreatedAt: backdated},
		outbox.Event{Key: "orders.2", Payload: []byte("b")},
	)

	stats, err := f.store.Stats(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(2), stats.Pending, "only the undispatched events count as backlog")
	require.Equal(t, int64(1), stats.DeadLettered, "the dead letter must show up as depth, not just a counter tick")
	require.Zero(t, stats.InProgress, "no event may be left locked once the cycle ends")
	require.Greater(t, stats.OldestPendingAge, time.Hour,
		"lag must be measured from the oldest waiting event")
}

// An empty outbox reports zeros rather than failing — the gauges have to be
// publishable before the first event is ever saved.
func TestLifecycle_StatsOnAnEmptyOutbox(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	stats, err := f.store.Stats(t.Context())
	require.NoError(t, err)
	require.Equal(t, outbox.Stats{}, stats)
}
