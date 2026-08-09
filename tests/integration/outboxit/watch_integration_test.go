// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/outbox"
)

const (
	// probeInterval is how long a single watcher probe is given to come back
	// before another one is saved. See fixture.awaitWatcher.
	probeInterval = 250 * time.Millisecond

	// uncommittedWindow is how long an uncommitted event is watched for before
	// concluding that nothing dispatched it. A negative assertion cannot be
	// proven by waiting, only made expensive to violate, so it is kept short —
	// the positive half of the same test (delivery right after commit) is what
	// shows the notification path was live throughout.
	uncommittedWindow = 2 * time.Second
)

// requireChangeStreams skips a test when the deployment cannot serve change
// streams. The compose fixture is a replica set, so this only fires against a
// hand-pointed MONGO_URI — where the watcher has nothing to say.
func (f *fixture) requireChangeStreams(tb testing.TB) {
	tb.Helper()

	supported, err := f.store.SupportsChangeStreams(tb.Context())
	require.NoError(tb, err)

	if !supported {
		tb.Skip("MongoDB deployment is not a replica set or sharded cluster — change streams unavailable")
	}
}

// startWatcher runs ob.Watch in the background and stops it on cleanup,
// asserting that cancellation was a clean shutdown rather than an error.
//
// Cleanup order matters: this runs before the fixture disconnects its client,
// because a watcher outliving its connection would report a torn stream instead
// of the clean exit being asserted.
func (f *fixture) startWatcher(tb testing.TB, ob *outbox.Outbox) {
	tb.Helper()

	ctx, cancel := context.WithCancel(tb.Context())
	done := make(chan error, 1)
	go func() { done <- ob.Watch(ctx) }()

	tb.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(tb, err, "cancellation must be a clean shutdown")
		case <-time.After(settleWindow):
			require.Fail(tb, "the watcher did not stop after cancellation")
		}
	})
}

// awaitWatcher blocks until the watcher's change stream is demonstrably live,
// then clears the recording it made proving it.
//
// The stream is opened asynchronously — Watch spawns it, and the open costs a
// topology probe plus an aggregate round trip — and a change stream only ever
// reports inserts that happen after it is listening. A test that saved once and
// waited would therefore hang whenever the save won that race. Probing until one
// comes back is the only signal the driver offers; there is no "stream ready"
// callback to wait on.
func (f *fixture) awaitWatcher(tb testing.TB, ob *outbox.Outbox) {
	tb.Helper()

	deadline := time.Now().Add(settleWindow)
	for probe := 0; ; probe++ {
		f.save(tb, ob, event("watch.probe", fmt.Sprintf(`{"probe":%d}`, probe)))

		if settled(func() bool { return f.recorder.Count() > 0 }, probeInterval) {
			f.recorder.Reset()
			return
		}
		if time.Now().After(deadline) {
			require.FailNow(tb, "the watcher never picked up a probe event",
				"saved %d probes without a single notification-driven dispatch", probe+1)
		}
	}
}

// settled reports whether cond became true within the given window.
//
// Unlike require.Eventually it returns a verdict instead of failing, which is
// what both a retry loop and a negative assertion need.
func settled(cond func() bool, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(samplingInterval)
	}
}

// The headline property: with no schedule and no manual cycle, the change stream
// alone carries a saved event through to its handler. Nothing below the store
// can show this — an in-memory store has no oplog to notice the insert.
func TestWatch_DeliversWithoutAPollCycle(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.requireChangeStreams(t)

	ob := f.newOutbox(t)
	f.startWatcher(t, ob)
	f.awaitWatcher(t, ob)

	saved := f.save(t, ob, event("billing.invoice.paid", `{"invoice":1}`))

	require.Eventually(t, func() bool {
		return f.recorder.Count() == 1
	}, settleWindow, samplingInterval,
		"the change stream never woke the dispatcher:\n%s", f.recorder.Timeline())

	require.Equal(t, []string{`{"invoice":1}`}, f.recorder.Payloads())

	doc := f.load(t, saved[0].Id)
	require.Equal(t, string(outbox.StatusSent), doc.Status)
	require.Equal(t, uint32(1), doc.Attempts)
	require.Empty(t, doc.LockToken, "a completed event must not keep its lease")
}

// A change stream reports an insert at commit, never before — which is what
// makes it safe to drive dispatch from. If it surfaced uncommitted writes, a
// notification could publish an event whose business transaction went on to
// abort, reintroducing the exact inconsistency the outbox exists to prevent.
func TestWatch_DeliversNothingUntilTheTransactionCommits(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.requireChangeStreams(t)

	ob := f.newOutbox(t)
	f.startWatcher(t, ob)
	f.awaitWatcher(t, ob)

	sess, err := f.client.StartSession()
	require.NoError(t, err)
	defer sess.EndSession(t.Context())

	_, err = sess.WithTransaction(t.Context(), func(sessCtx context.Context) (any, error) {
		if saveErr := ob.Save(sessCtx, event("billing.order.created", `{"order":"o-1"}`)); saveErr != nil {
			return nil, saveErr
		}

		require.False(t, settled(func() bool { return f.recorder.Count() > 0 }, uncommittedWindow),
			"an uncommitted event must not be dispatched:\n%s", f.recorder.Timeline())

		return nil, nil
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return f.recorder.Count() == 1
	}, settleWindow, samplingInterval,
		"the commit never produced a notification:\n%s", f.recorder.Timeline())

	require.Equal(t, []string{`{"order":"o-1"}`}, f.recorder.Payloads())
}

// One transaction can save far more events than a batch holds, and the inserts
// that reported them are already spent by the time the first cycle returns.
// Without the full-batch re-arm the remainder would sit untouched until the next
// scheduled tick — which, in a watcher-driven deployment, may be a long way off.
func TestWatch_DrainsBacklogLargerThanOneBatch(t *testing.T) {
	t.Parallel()

	const (
		batchSize = 2
		events    = 7
	)

	f := newFixture(t)
	f.requireChangeStreams(t)

	ob := f.newOutbox(t, outbox.WithEventsBatchSize(batchSize))
	f.startWatcher(t, ob)
	f.awaitWatcher(t, ob)

	batch := make([]outbox.Event, 0, events)
	for i := range events {
		batch = append(batch, event(fmt.Sprintf("orders.%d", i), fmt.Sprintf(`{"seq":%d}`, i)))
	}
	saved := f.save(t, ob, batch...)

	require.Eventually(t, func() bool {
		return f.recorder.Count() == events
	}, settleWindow, samplingInterval,
		"a backlog of %d events did not drain through a batch size of %d:\n%s",
		events, batchSize, f.recorder.Timeline())

	require.Empty(t, f.recorder.Duplicates(),
		"re-arming the drain must not re-deliver an event:\n%s", f.recorder.Timeline())

	for _, ev := range saved {
		require.Equal(t, string(outbox.StatusSent), f.load(t, ev.Id).Status)
	}
}

// The shape every real deployment has: one replica running a watcher while the
// others poll on their schedule. The two drivers race for the same events, and
// only the store's transactional lock keeps a notification-driven fetch and a
// scheduled one from handing the same event to two handlers.
func TestWatch_CoexistsWithPollingDispatchers(t *testing.T) {
	t.Parallel()

	const (
		batchSize = 5
		events    = 40
		cycles    = 40
	)

	f := newFixture(t)
	f.requireChangeStreams(t)

	watched := f.newOutbox(t, outbox.WithEventsBatchSize(batchSize))
	f.startWatcher(t, watched)
	f.awaitWatcher(t, watched)

	// A second instance of the same service, driven by its schedule only. It
	// shares the recorder, so a duplicate shows up as two entries for one ID
	// regardless of which driver produced them.
	poller := f.newOutbox(t, outbox.WithEventsBatchSize(batchSize))

	batch := make([]outbox.Event, 0, events)
	for i := range events {
		batch = append(batch, event(fmt.Sprintf("orders.%d", i), fmt.Sprintf(`{"seq":%d}`, i)))
	}
	saved := f.save(t, watched, batch...)

	var wg sync.WaitGroup
	wg.Go(func() {
		for range cycles {
			require.NoError(t, poller.RunDispatchCycle(t.Context()))
		}
	})
	wg.Wait()

	require.Eventually(t, func() bool {
		return f.recorder.Count() == events
	}, settleWindow, samplingInterval,
		"the two drivers together did not deliver every event:\n%s", f.recorder.Timeline())

	require.Empty(t, f.recorder.Duplicates(),
		"a watcher and a poller must not hand the same event to two handlers:\n%s", f.recorder.Timeline())

	for _, ev := range saved {
		require.Equal(t, string(outbox.StatusSent), f.load(t, ev.Id).Status)
	}
}

// The capability probe, against a deployment that genuinely has the capability.
// The unit tests can only assert how the reply is interpreted; whether a real
// replica set answers hello the way the probe expects is decided here.
func TestWatch_SupportsChangeStreamsOnAReplicaSet(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	supported, err := f.store.SupportsChangeStreams(t.Context())
	require.NoError(t, err)
	require.True(t, supported,
		"the fixture runs transactions, so it is a replica set — change streams must be reported as available")
}
