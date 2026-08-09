// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/outbox"

	"github.com/altessa-s/go-atlas/tests/integration/outboxit"
)

// Several instances of a service poll the same collection. The fetch takes its
// lock inside a transaction precisely so two of them cannot claim one event —
// a property that does not exist until real concurrent transactions are racing.
func TestConcurrency_EachEventIsDeliveredOnce(t *testing.T) {
	t.Parallel()

	const (
		dispatchers = 4
		events      = 60
	)

	f := newFixture(t)

	writer := f.newOutbox(t)
	batch := make([]outbox.Event, 0, events)
	for i := range events {
		batch = append(batch, event(fmt.Sprintf("orders.%d", i), fmt.Sprintf(`{"seq":%d}`, i)))
	}
	saved := f.save(t, writer, batch...)

	// Every dispatcher shares one recorder, so a duplicate shows up as two
	// entries for the same ID no matter which instance made them.
	var wg sync.WaitGroup
	for range dispatchers {
		ob := f.newOutbox(t, outbox.WithEventsBatchSize(7))
		wg.Go(func() {
			// Loop rather than run once: a single cycle per dispatcher would
			// leave the tail undelivered and the race barely exercised.
			for range 40 {
				require.NoError(t, ob.RunDispatchCycle(t.Context()))
			}
		})
	}
	wg.Wait()

	require.Empty(t, f.recorder.Duplicates(),
		"no event may be handed to two dispatchers:\n%s", f.recorder.Timeline())
	require.Equal(t, events, f.recorder.Count(), "every event must be delivered exactly once")

	for _, ev := range saved {
		require.Equal(t, string(outbox.StatusSent), f.load(t, ev.Id).Status)
	}
}

// The fencing scenario. A dispatcher stalls long enough for the unlock sweeper
// to reclaim its event; a second dispatcher takes it over and publishes it. When
// the first one finally comes back with a failure, its write must be refused —
// otherwise it would overwrite a delivered event with a failed one and the
// outbox would publish it a second time.
func TestConcurrency_ReclaimedLeaseCannotOverwriteTheNewerResult(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	release := make(chan struct{})
	stalled := make(chan struct{})
	var once sync.Once

	// The slow dispatcher: blocks on its first delivery, then reports failure.
	slow := f.newOutbox(t)
	f.recorder.Respond(func(outboxit.Delivery) error {
		once.Do(func() { close(stalled) })
		<-release
		return errors.New("the destination never answered")
	})

	saved := f.save(t, slow, event("billing.invoice.paid", `{"invoice":1}`))
	id := saved[0].Id

	var wg sync.WaitGroup
	wg.Go(func() { require.NoError(t, slow.RunDispatchCycle(t.Context())) })

	// Wait until the event is locked and the handler is inside the call.
	select {
	case <-stalled:
	case <-time.After(settleWindow):
		close(release)
		wg.Wait()
		t.Fatal("the first dispatcher never reached the handler")
	}

	locked := f.load(t, id)
	require.Equal(t, string(outbox.StatusInProgress), locked.Status)
	require.NotEmpty(t, locked.LockToken, "a locked event must carry a lease token")

	// Reclaim the lease out from under it, exactly as the unlock cycle would
	// for a dispatcher that died. lockExpiry=0 makes every held lock stuck.
	require.NoError(t, f.store.UnlockStuckEvents(t.Context(), 0))

	reclaimed := f.load(t, id)
	require.Equal(t, string(outbox.StatusPending), reclaimed.Status)
	require.Empty(t, reclaimed.LockToken, "reclaiming must invalidate the old lease")

	// A second dispatcher picks it up and succeeds.
	takeover := f.newOutbox(t)
	f.recorder.Respond(nil)
	require.NoError(t, takeover.RunDispatchCycle(t.Context()))
	require.Equal(t, string(outbox.StatusSent), f.load(t, id).Status)

	// Only now let the stalled dispatcher finish and try to record its failure.
	close(release)
	wg.Wait()

	final := f.load(t, id)
	require.Equal(t, string(outbox.StatusSent), final.Status,
		"a write from a revoked lease must not resurrect a delivered event:\n%s", f.recorder.Timeline())
	require.Nil(t, final.LastError, "the stale failure must not be recorded either")
}

// The complement: the sweeper must leave a healthy dispatcher alone. With a lock
// time above the handle timeout, an event in flight is never reclaimed, so the
// duplicate delivery the previous scenario tolerates does not happen routinely.
func TestConcurrency_SweeperLeavesLiveLeasesAlone(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	release := make(chan struct{})
	stalled := make(chan struct{})
	var once sync.Once

	ob := f.newOutbox(t)
	f.recorder.Respond(func(outboxit.Delivery) error {
		once.Do(func() { close(stalled) })
		<-release
		return nil
	})

	saved := f.save(t, ob, event("billing.invoice.paid", `{"invoice":1}`))

	var wg sync.WaitGroup
	wg.Go(func() { require.NoError(t, ob.RunDispatchCycle(t.Context())) })

	select {
	case <-stalled:
	case <-time.After(settleWindow):
		close(release)
		wg.Wait()
		t.Fatal("the dispatcher never reached the handler")
	}

	// The configured lock time is well above what this delivery has taken.
	require.NoError(t, ob.RunUnlockCycle(t.Context()))
	require.Equal(t, string(outbox.StatusInProgress), f.load(t, saved[0].Id).Status,
		"a lease that has not expired must survive the sweeper")

	close(release)
	wg.Wait()

	require.Equal(t, string(outbox.StatusSent), f.load(t, saved[0].Id).Status)
	require.Equal(t, 1, f.recorder.Count(), "the event must not be delivered twice:\n%s", f.recorder.Timeline())
}

// Overlapping cycles on one instance collapse into a single execution, so a
// scheduler firing faster than the store can drain does not multiply the load.
func TestConcurrency_OverlappingCyclesOnOneInstanceCollapse(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	release := make(chan struct{})
	stalled := make(chan struct{})
	var once sync.Once

	ob := f.newOutbox(t)
	f.recorder.Respond(func(outboxit.Delivery) error {
		once.Do(func() { close(stalled) })
		<-release
		return nil
	})

	f.save(t, ob, event("billing.invoice.paid", `{"invoice":1}`))

	var wg sync.WaitGroup
	wg.Go(func() { require.NoError(t, ob.RunDispatchCycle(t.Context())) })

	select {
	case <-stalled:
	case <-time.After(settleWindow):
		close(release)
		wg.Wait()
		t.Fatal("the dispatcher never reached the handler")
	}

	// A second cycle while the first is still in flight must be a no-op.
	require.NoError(t, ob.RunDispatchCycle(t.Context()))

	close(release)
	wg.Wait()

	require.Equal(t, 1, f.recorder.Count(),
		"an overlapping cycle must not start a second pass:\n%s", f.recorder.Timeline())
}
