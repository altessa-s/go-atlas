// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/transport/broker/msg"

	brokeroutbox "github.com/altessa-s/go-atlas/transport/broker/outbox"
	natsprovider "github.com/altessa-s/go-atlas/transport/broker/providers/nats"
)

const (
	// dedupWindow is the JetStream duplicate window for the test stream. It only
	// has to outlive one test, but it must be explicit: the default is two
	// minutes and a test that silently depended on it would keep passing if the
	// publisher stopped sending an ID at all.
	dedupWindow = time.Minute

	// subjectPrefix is the base of the per-test subject. JetStream refuses to
	// create two streams whose subjects overlap, so parallel tests cannot share
	// one — each gets its own leaf token.
	subjectPrefix = "billing.invoice.paid."
)

// natsURL returns the broker address, matching tests/integration/docker-compose.yml.
func natsURL() string {
	if v := os.Getenv("NATS_URL"); v != "" {
		return v
	}
	return "nats://127.0.0.1:14222"
}

// newStream connects to JetStream and creates a throwaway stream with an
// explicit duplicate window, returning the subject it captures. Skips when NATS
// is unreachable, as the rest of the suite does for MongoDB.
func newStream(tb testing.TB) (*natsprovider.Nats, jetstream.Stream, string) {
	tb.Helper()

	nc, err := nats.Connect(natsURL(), nats.Timeout(2*time.Second), nats.RetryOnFailedConnect(false))
	if err != nil {
		tb.Skipf("NATS unreachable at %s (%v) — start it with: docker compose -f tests/integration/docker-compose.yml up -d --wait nats", natsURL(), err)
	}
	tb.Cleanup(nc.Close)

	js, err := jetstream.New(nc)
	require.NoError(tb, err)

	// One name, one subject leaf, both from the per-test uniqueness rule.
	name := databaseName(tb)
	subject := subjectPrefix + name

	stream, err := js.CreateStream(tb.Context(), jetstream.StreamConfig{
		Name:       name,
		Subjects:   []string{subject},
		Duplicates: dedupWindow,
		Storage:    jetstream.MemoryStorage,
	})
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = js.DeleteStream(context.Background(), name) })

	provider, err := natsprovider.New(nc)
	require.NoError(tb, err)

	return provider, stream, subject
}

// messageCount returns how many messages the stream currently holds.
func messageCount(tb testing.TB, stream jetstream.Stream) uint64 {
	tb.Helper()

	info, err := stream.Info(tb.Context())
	require.NoError(tb, err)

	return info.State.Msgs
}

// The outbox publishes at-least-once, so a message can reach the broker and the
// status write can still be lost — after which the event is republished. Whether
// that repeat reaches consumers depends on the broker collapsing it, and that
// only works if every attempt carries the same identity.
//
// This is the end of the chain the unit tests cover in pieces: the adapter
// defaults the deduplication ID to the outbox event ID, the NATS provider maps
// it onto Nats-Msg-Id, and JetStream drops the repeat. Nothing below the adapter
// is stubbed here.
func TestDedup_RepublishedEventIsCollapsedByJetStream(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	provider, stream, subject := newStream(t)

	ob := brokeroutbox.New(f.store, provider,
		brokeroutbox.WithHandleTimeout(handleTimeout),
		brokeroutbox.WithMaxLockTime(lockTime),
	)

	require.NoError(t, ob.Publish(t.Context(), *msg.NewMessage(subject, []byte(`{"invoice":1}`))))
	require.NoError(t, ob.RunDispatchCycle(t.Context()))
	require.Equal(t, uint64(1), messageCount(t, stream), "the first dispatch must reach the stream")

	events := f.loadAll(t)
	require.Len(t, events, 1)
	require.Equal(t, "sent", events[0].Status)

	// Model the lost status write: the publish succeeded, the process died
	// before recording it, and the unlock sweeper handed the event back. The
	// event keeps its ID, which is the whole point.
	_, err := f.events.UpdateByID(t.Context(), events[0].ID, bson.M{"$set": bson.M{
		"status":       "pending",
		"published_at": nil,
		"lock_token":   nil,
	}})
	require.NoError(t, err)

	require.NoError(t, ob.RunDispatchCycle(t.Context()))
	require.Equal(t, "sent", f.load(t, events[0].ID).Status, "the republish must succeed from the outbox's view")

	require.Equal(t, uint64(1), messageCount(t, stream),
		"JetStream must collapse the republish: the outbox reuses the event ID as Nats-Msg-Id")
}

// The complement, and the reason the assertion above is meaningful: two
// genuinely different events are two messages. A deduplication ID that was
// constant — or a stream that deduplicated on content — would pass the first
// test and fail this one.
func TestDedup_DistinctEventsAreNotCollapsed(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	provider, stream, subject := newStream(t)

	ob := brokeroutbox.New(f.store, provider,
		brokeroutbox.WithHandleTimeout(handleTimeout),
		brokeroutbox.WithMaxLockTime(lockTime),
	)

	require.NoError(t, ob.PublishBatch(t.Context(),
		*msg.NewMessage(subject, []byte(`{"invoice":1}`)),
		*msg.NewMessage(subject, []byte(`{"invoice":2}`)),
	))
	require.NoError(t, ob.RunDispatchCycle(t.Context()))

	require.Equal(t, uint64(2), messageCount(t, stream),
		"distinct events must not be deduplicated against each other")
}

// A caller-supplied deduplication ID collapses repeats across producers, not
// only across one event's retries — so the same business key published as two
// separate outbox events must still reach the stream once.
func TestDedup_CallerSuppliedIdCollapsesSeparateEvents(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	provider, stream, subject := newStream(t)

	ob := brokeroutbox.New(f.store, provider,
		brokeroutbox.WithHandleTimeout(handleTimeout),
		brokeroutbox.WithMaxLockTime(lockTime),
	)

	withBusinessKey := func() msg.Message {
		return *msg.NewMessageWithMeta(subject, []byte(`{"invoice":7}`),
			msg.Meta{{Key: msg.MetaKeyDeduplicateId, Value: "invoice-7-paid"}})
	}

	require.NoError(t, ob.Publish(t.Context(), withBusinessKey()))
	require.NoError(t, ob.Publish(t.Context(), withBusinessKey()))
	require.NoError(t, ob.RunDispatchCycle(t.Context()))

	require.Len(t, f.loadAll(t), 2, "both events must have been stored")
	require.Equal(t, uint64(1), messageCount(t, stream),
		"one business key must yield one message, however many events carried it")
}
