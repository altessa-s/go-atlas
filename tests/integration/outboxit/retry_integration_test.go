// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/outbox"

	"github.com/altessa-s/go-atlas/tests/integration/outboxit"
)

// errBrokerDown stands in for a transient destination failure.
var errBrokerDown = errors.New("broker unavailable")

// A failed attempt must be written back. This is the regression guard for a bug
// that only a real store makes visible: the failure verdict travelled out of the
// concurrent batch helper as an error, and that helper drops the transformed
// value of any item whose function failed — so the attempt count never grew, the
// error was never recorded, and no event could ever reach a terminal status.
// Every unit-level assertion still passed, because they inspected the event the
// outbox had built rather than the document that was stored.
func TestRetry_FailedAttemptIsPersisted(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ob := f.newOutbox(t)
	f.recorder.Respond(func(outboxit.Delivery) error { return errBrokerDown })

	saved := f.save(t, ob, event("billing.invoice.paid", `{"invoice":1}`))
	require.NoError(t, ob.RunDispatchCycle(t.Context()))

	doc := f.load(t, saved[0].Id)
	require.Equal(t, string(outbox.StatusFailed), doc.Status)
	require.Equal(t, uint32(1), doc.Attempts, "the attempt must be counted")
	require.NotNil(t, doc.LastError, "the failure reason must be readable from the store")
	require.Contains(t, *doc.LastError, errBrokerDown.Error())
	require.NotNil(t, doc.NextAttemptAt, "a retryable failure must carry its backoff deadline")
	require.Empty(t, doc.LockToken, "the lease must be released even on failure")
}

// The backoff is enforced by the store against its own clock, so a retry cannot
// be pulled forward by an eager dispatcher — and it survives a restart, because
// the deadline lives in the document rather than in a process.
func TestRetry_BackoffWithholdsTheEventUntilItElapses(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ob := f.newOutbox(t)
	f.recorder.Respond(func(outboxit.Delivery) error { return errBrokerDown })

	saved := f.save(t, ob, event("billing.invoice.paid", `{"invoice":1}`))

	require.NoError(t, ob.RunDispatchCycle(t.Context()))
	require.Equal(t, 1, f.recorder.Count())

	// Immediately afterwards the event is still serving its backoff.
	require.NoError(t, ob.RunDispatchCycle(t.Context()))
	require.Equal(t, 1, f.recorder.Count(),
		"a cycle run inside the backoff window must not pick the event up:\n%s", f.recorder.Timeline())
	require.Equal(t, string(outbox.StatusFailed), f.load(t, saved[0].Id).Status)

	// A fresh outbox proves the deadline is durable rather than in-process.
	restarted := f.newOutbox(t)
	require.Eventually(t, func() bool {
		require.NoError(t, restarted.RunDispatchCycle(t.Context()))
		return f.recorder.Count() >= 2
	}, settleWindow, samplingInterval, "the retry never became eligible:\n%s", f.recorder.Timeline())

	require.Equal(t, uint32(2), f.load(t, saved[0].Id).Attempts)
}

// The budget is spent one attempt per cycle, and the event is dead-lettered
// when it runs out — not retried forever, which is what turns one poison
// message into a permanently stuck queue.
func TestRetry_DeadLettersWhenTheBudgetRunsOut(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ob := f.newOutbox(t,
		outbox.WithRetryMaxAttempts(2),
		// Effectively no waiting between the two attempts this test needs.
		outbox.WithRetryBaseDelay(time.Millisecond),
		outbox.WithRetryMaxDelay(10*time.Millisecond),
	)
	f.recorder.Respond(func(outboxit.Delivery) error { return errBrokerDown })

	saved := f.save(t, ob, event("billing.invoice.paid", `{"invoice":1}`))

	require.Eventually(t, func() bool {
		require.NoError(t, ob.RunDispatchCycle(t.Context()))
		return f.load(t, saved[0].Id).Status == string(outbox.StatusMaxAttemptReached)
	}, settleWindow, samplingInterval, "the event never exhausted its budget:\n%s", f.recorder.Timeline())

	doc := f.load(t, saved[0].Id)
	require.Equal(t, uint32(2), doc.Attempts, "exactly the configured budget, no more")
	require.Equal(t, 2, f.recorder.Count(), "one delivery attempt per cycle:\n%s", f.recorder.Timeline())
	require.Nil(t, doc.NextAttemptAt, "a dead-lettered event must not be rescheduled")

	// Further cycles must leave it alone.
	require.NoError(t, ob.RunDispatchCycle(t.Context()))
	require.Equal(t, 2, f.recorder.Count(), "a dead-lettered event must never be picked up again")
}

// A failure the predicate calls permanent is dead-lettered on the spot. Burning
// the whole budget on a malformed payload only delays the operator seeing it.
func TestRetry_PermanentFailureIsRejectedWithoutRetrying(t *testing.T) {
	t.Parallel()

	permanent := errors.New("payload is not valid for this transport")

	f := newFixture(t)
	ob := f.newOutbox(t,
		outbox.WithRetryMaxAttempts(10),
		outbox.WithShouldRetry(func(err error) bool { return !errors.Is(err, permanent) }),
	)
	f.recorder.Respond(func(outboxit.Delivery) error { return permanent })

	saved := f.save(t, ob, event("billing.invoice.paid", "not-json"))

	require.NoError(t, ob.RunDispatchCycle(t.Context()))

	doc := f.load(t, saved[0].Id)
	require.Equal(t, string(outbox.StatusRejected), doc.Status)
	require.Equal(t, uint32(1), doc.Attempts, "a permanent failure must cost one attempt, not ten")
	require.Nil(t, doc.NextAttemptAt)
	require.NotNil(t, doc.LastError)

	require.NoError(t, ob.RunDispatchCycle(t.Context()))
	require.Equal(t, 1, f.recorder.Count(), "a rejected event must never be retried:\n%s", f.recorder.Timeline())
}

// A destination that recovers mid-flight must see the event through. The retry
// carries the same event ID as the first attempt, which is what lets a broker
// with deduplication collapse the two.
func TestRetry_SucceedsOnceTheDestinationRecovers(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ob := f.newOutbox(t,
		outbox.WithRetryBaseDelay(50*time.Millisecond),
		outbox.WithRetryMaxDelay(200*time.Millisecond),
	)

	// Atomic because the handler runs on the dispatcher's goroutines while the
	// assertion loop drives cycles from another.
	var down atomic.Bool
	down.Store(true)
	f.recorder.Respond(func(outboxit.Delivery) error {
		if down.CompareAndSwap(true, false) {
			return errBrokerDown
		}
		return nil
	})

	saved := f.save(t, ob, event("billing.invoice.paid", `{"invoice":1}`))

	require.Eventually(t, func() bool {
		require.NoError(t, ob.RunDispatchCycle(t.Context()))
		return f.load(t, saved[0].Id).Status == string(outbox.StatusSent)
	}, settleWindow, samplingInterval, "the event never went through:\n%s", f.recorder.Timeline())

	doc := f.load(t, saved[0].Id)
	require.Equal(t, uint32(2), doc.Attempts)
	require.Nil(t, doc.LastError, "a successful attempt must clear the recorded failure")

	deliveries := f.recorder.Deliveries()
	require.Len(t, deliveries, 2)
	require.Equal(t, deliveries[0].EventID, deliveries[1].EventID,
		"a retry must reuse the event ID so the destination can deduplicate it")
}
