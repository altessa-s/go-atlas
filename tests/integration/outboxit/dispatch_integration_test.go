// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/data/outbox"
)

// The baseline: an event saved through a committed transaction is handed to the
// handler on the next cycle and ends up marked sent, with its lock released.
func TestDispatch_DeliversSavedEvent(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ob := f.newOutbox(t)

	saved := f.save(t, ob, event("billing.invoice.paid", `{"invoice":1}`))

	require.NoError(t, ob.RunDispatchCycle(t.Context()))

	require.Equal(t, []string{`{"invoice":1}`}, f.recorder.Payloads(), f.recorder.Timeline())

	doc := f.load(t, saved[0].Id)
	require.Equal(t, string(outbox.StatusSent), doc.Status)
	require.Equal(t, uint32(1), doc.Attempts)
	require.NotNil(t, doc.PublishedAt)
	require.Nil(t, doc.LastError)
	require.Empty(t, doc.LockToken, "a completed event must not keep its lease")
}

// The property the whole pattern exists for: business data and its event commit
// together. An in-memory store cannot show this, because there is no shared
// transaction to commit.
func TestDispatch_CommittedTransactionPublishesExactlyOnce(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ob := f.newOutbox(t)

	f.inTransaction(t, func(sessCtx context.Context) error {
		if _, err := f.orders.InsertOne(sessCtx, bson.M{"_id": "order-1", "total": 42}); err != nil {
			return err
		}
		return ob.Save(sessCtx, event("billing.order.created", `{"order":"order-1"}`))
	})

	require.Equal(t, int64(1), f.countOrders(t))
	require.Equal(t, int64(1), f.countEvents(t))

	require.NoError(t, ob.RunDispatchCycle(t.Context()))
	require.Equal(t, []string{`{"order":"order-1"}`}, f.recorder.Payloads())
}

// The other half of the same property, and the failure mode that makes a naive
// publish-after-commit unsafe in reverse: a rolled-back business write must not
// leave an event that a dispatcher would happily publish afterwards.
func TestDispatch_AbortedTransactionPublishesNothing(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ob := f.newOutbox(t)

	rollback := errors.New("business rule rejected the order")

	sess, err := f.client.StartSession()
	require.NoError(t, err)
	defer sess.EndSession(t.Context())

	_, err = sess.WithTransaction(t.Context(), func(sessCtx context.Context) (any, error) {
		if _, insErr := f.orders.InsertOne(sessCtx, bson.M{"_id": "order-2", "total": 7}); insErr != nil {
			return nil, insErr
		}
		if saveErr := ob.Save(sessCtx, event("billing.order.created", `{"order":"order-2"}`)); saveErr != nil {
			return nil, saveErr
		}
		return nil, rollback // Abort after both writes.
	})
	require.ErrorIs(t, err, rollback)

	require.Zero(t, f.countOrders(t), "the aborted business write must be gone")
	require.Zero(t, f.countEvents(t), "the event must roll back with it")

	require.NoError(t, ob.RunDispatchCycle(t.Context()))
	require.Zero(t, f.recorder.Count(), "nothing was committed, so nothing may be published")
}

// Save validates the whole batch before it writes anything, so a rejected event
// leaves the caller's transaction clean and still committable. If validation
// happened mid-write, the caller would be forced to abort work that was fine.
func TestDispatch_RejectedSaveLeavesTheTransactionCommittable(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ob := f.newOutbox(t)

	var saveErr error
	f.inTransaction(t, func(sessCtx context.Context) error {
		if _, err := f.orders.InsertOne(sessCtx, bson.M{"_id": "order-3", "total": 1}); err != nil {
			return err
		}
		// The second event is invalid; neither may reach the store.
		saveErr = ob.Save(sessCtx,
			event("billing.order.created", `{"order":"order-3"}`),
			event("", `{"order":"order-3"}`),
		)
		return nil // Commit anyway — the point is that we still can.
	})

	require.ErrorIs(t, saveErr, outbox.ErrEmptyKey)
	require.Equal(t, int64(1), f.countOrders(t), "the valid business write must survive")
	require.Zero(t, f.countEvents(t), "a rejected batch must write no event at all")
}

// Compaction collapses a burst of updates for one entity down to the newest
// one. The superseded events must be recorded as skipped rather than dropped,
// so the collection still explains what happened to them.
func TestDispatch_CompactionPublishesOnlyTheLatestPerKey(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ob := f.newOutbox(t, outbox.WithCompaction())

	saved := f.save(t, ob,
		event("orders.42", `{"state":"pending"}`),
		event("orders.42", `{"state":"processing"}`),
		event("orders.42", `{"state":"completed"}`),
		event("orders.99", `{"state":"pending"}`),
	)

	require.NoError(t, ob.RunDispatchCycle(t.Context()))

	require.ElementsMatch(t,
		[]string{`{"state":"completed"}`, `{"state":"pending"}`},
		f.recorder.Payloads(),
		f.recorder.Timeline())

	require.Equal(t, string(outbox.StatusSkipped), f.load(t, saved[0].Id).Status)
	require.Equal(t, string(outbox.StatusSkipped), f.load(t, saved[1].Id).Status)
	require.Equal(t, string(outbox.StatusSent), f.load(t, saved[2].Id).Status)
	require.Equal(t, string(outbox.StatusSent), f.load(t, saved[3].Id).Status)
}

// A batch larger than the configured size is drained across cycles, and each
// cycle selects the oldest events still waiting — the ordering the store
// contract promises and compaction depends on.
//
// The assertion is on which events a cycle selected, not on the order they were
// handed over: a batch is dispatched concurrently, so the delivery order within
// one cycle is deliberately unspecified.
func TestDispatch_SelectsOldestEventsFirstAcrossBatches(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ob := f.newOutbox(t, outbox.WithEventsBatchSize(2))

	f.save(t, ob,
		event("k.1", "first"),
		event("k.2", "second"),
		event("k.3", "third"),
		event("k.4", "fourth"),
	)

	require.NoError(t, ob.RunDispatchCycle(t.Context()))
	require.ElementsMatch(t, []string{"first", "second"}, f.recorder.Payloads(),
		"the first cycle must take the two oldest events:\n%s", f.recorder.Timeline())

	require.NoError(t, ob.RunDispatchCycle(t.Context()))
	require.ElementsMatch(t, []string{"first", "second", "third", "fourth"}, f.recorder.Payloads(),
		"the second cycle must take the remaining two:\n%s", f.recorder.Timeline())
}
