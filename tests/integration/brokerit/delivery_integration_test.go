// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package brokerit_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/tests/integration/brokerit"
	"github.com/altessa-s/go-atlas/transport/broker"
	"github.com/altessa-s/go-atlas/transport/broker/inprogress"
	"github.com/altessa-s/go-atlas/transport/broker/msg"

	natsprovider "github.com/altessa-s/go-atlas/transport/broker/providers/nats"
)

// The baseline: a message published through the broker reaches an ephemeral
// subscriber with its payload, its subject and every application header, and
// none of the transport's own Nats-* headers leak into the metadata.
func TestDelivery_RoundTripCarriesDataAndHeaders(t *testing.T) {
	t.Parallel()

	w := newWorld(t)

	got := make(chan *msg.Message, 1)
	subscribe(t, w.provider, nil, brokerit.NewRecorder(w.subject, func(_ context.Context, m *msg.Message, _ int) {
		_ = m.Ack()
		got <- m
	}))

	sent := msg.NewMessageWithMeta(w.subject, []byte(`{"order":42}`), msg.Meta{
		{Key: "tenant", Value: "acme"},
		{Key: "traceparent", Value: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"},
		{Key: msg.MetaKeyMessageId, Value: "order-42"},
	})
	require.NoError(t, broker.New(w.provider).Publish(t.Context(), *sent))

	var m *msg.Message
	select {
	case m = <-got:
	case <-time.After(settleWindow):
		t.Fatal("the message never reached the subscriber")
	}

	require.Equal(t, w.subject, m.Topic)
	require.JSONEq(t, `{"order":42}`, string(m.Data))

	meta := m.Metadata.Map()
	require.Equal(t, "acme", meta["tenant"])
	require.Equal(t, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", meta["traceparent"])
	require.Equal(t, "order-42", msg.MessageIdFromMeta(m.Metadata))

	created, err := msg.MessageCreatedTimeFromMeta(m.Metadata)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now(), created, time.Minute, "the creation stamp must survive the wire")

	for k := range meta {
		require.False(t, strings.HasPrefix(k, "Nats-"), "transport header %q leaked into the metadata", k)
	}
}

// A batch is pipelined to the server; every message must arrive exactly once,
// be acknowledged, and leave the durable consumer with nothing outstanding and
// nothing redelivered.
func TestDelivery_BatchArrivesOnceAndSettles(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	consumer := uniqueName(t)
	rec := brokerit.NewRecorder(w.subject, brokerit.Ack)
	subscribe(t, w.provider, natsprovider.SubscriberWithConsumer(w.durable(consumer, 10*time.Second)), rec)

	want := payloads("batch", 50)
	batch := make([]msg.Message, 0, len(want))
	for _, p := range want {
		batch = append(batch, *msg.NewMessage(w.subject, []byte(p)))
	}
	require.NoError(t, broker.New(w.provider).PublishBatch(t.Context(), batch...))

	info := w.awaitSettled(t, consumer)
	require.Equal(t, uint64(len(want)), info.Delivered.Consumer, "the server delivered more than once per message")
	require.Equal(t, len(want), rec.Count(), "every message exactly once")
	for _, p := range want {
		require.Equal(t, 1, rec.Attempts(p), "payload %s", p)
	}
}

// NakWithBackOff asks the server to hold the message back for the delay the
// schedule gives the current delivery count. The gaps between deliveries must
// follow the schedule — and stay well below the ack wait, so the redelivery is
// the Nak's and not an expired ack timer's.
func TestDelivery_NakWithBackOffWaitsTheScheduledDelay(t *testing.T) {
	t.Parallel()

	const longAckWait = 20 * time.Second
	schedule := []time.Duration{400 * time.Millisecond, 900 * time.Millisecond}

	w := newWorld(t)
	consumer := uniqueName(t)
	rec := brokerit.NewRecorder(w.subject, func(_ context.Context, m *msg.Message, attempt int) {
		if attempt <= len(schedule) {
			_ = m.NakWithBackOff(msg.ScheduleBackOff(schedule...))
			return
		}
		_ = m.Ack()
	})
	subscribe(t, w.provider, natsprovider.SubscriberWithConsumer(w.durable(consumer, longAckWait)), rec)

	w.publish(t, "retry-me")
	w.awaitSettled(t, consumer)

	got := rec.DeliveriesOf("retry-me")
	require.Len(t, got, len(schedule)+1)
	for i, delay := range schedule {
		gap := got[i+1].At.Sub(got[i].At)
		require.GreaterOrEqual(t, gap, delay, "delivery %d came back before its backoff", i+2)
		require.Less(t, gap, delay+longAckWait/2, "delivery %d waited for the ack timer, not the backoff", i+2)
	}
}

// A plain Nak redelivers at once: the second delivery must arrive long before
// the ack wait would have expired.
func TestDelivery_NakRedeliversImmediately(t *testing.T) {
	t.Parallel()

	const longAckWait = 20 * time.Second

	w := newWorld(t)
	consumer := uniqueName(t)
	rec := brokerit.NewRecorder(w.subject, func(_ context.Context, m *msg.Message, attempt int) {
		if attempt == 1 {
			_ = m.Nak()
			return
		}
		_ = m.Ack()
	})
	subscribe(t, w.provider, natsprovider.SubscriberWithConsumer(w.durable(consumer, longAckWait)), rec)

	w.publish(t, "nak-me")
	info := w.awaitSettled(t, consumer)

	got := rec.DeliveriesOf("nak-me")
	require.Len(t, got, 2)
	require.Less(t, got[1].At.Sub(got[0].At), longAckWait/2, "a Nak must not wait for the ack timer")
	require.Equal(t, uint64(2), info.Delivered.Consumer, "the server must count the redelivery")
}

// advisory is the part of a JetStream consumer advisory the scenarios read.
type advisory struct {
	StreamSeq  uint64 `json:"stream_seq"`
	Deliveries uint64 `json:"deliveries"`
	Reason     string `json:"reason"`
}

// watchAdvisory subscribes to one JetStream consumer advisory for the given
// stream and consumer. The subscription is flushed before returning, so an
// advisory raised after this call cannot be missed.
func watchAdvisory(tb testing.TB, nc *nats.Conn, kind, stream, consumer string) <-chan *nats.Msg {
	tb.Helper()

	ch := make(chan *nats.Msg, 8)
	sub, err := nc.ChanSubscribe("$JS.EVENT.ADVISORY.CONSUMER."+kind+"."+stream+"."+consumer, ch)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = sub.Unsubscribe() })
	require.NoError(tb, nc.Flush())

	return ch
}

// awaitAdvisory decodes the next advisory from ch.
func awaitAdvisory(tb testing.TB, ch <-chan *nats.Msg) advisory {
	tb.Helper()

	select {
	case m := <-ch:
		var a advisory
		require.NoError(tb, json.Unmarshal(m.Data, &a))
		return a
	case <-time.After(settleWindow):
		tb.Fatal("no advisory raised")
		return advisory{}
	}
}

// Term is the handler's verdict that a message can never succeed. The server
// must stop delivering it — even past the ack wait, when an unacknowledged
// message would otherwise come back — and the reason must reach the
// terminated advisory, where an operator or a dead-letter sink picks it up.
func TestDelivery_TermStopsRedeliveryAndReportsTheReason(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	consumer := uniqueName(t)
	terminated := watchAdvisory(t, w.provider.NATSConn(), "MSG_TERMINATED", w.name, consumer)

	rec := brokerit.NewRecorder(w.subject, func(_ context.Context, m *msg.Message, _ int) {
		_ = m.Term("schema v1 is no longer supported")
	})
	subscribe(t, w.provider, natsprovider.SubscriberWithConsumer(w.durable(consumer, ackWait)), rec)

	w.publish(t, "poison")

	adv := awaitAdvisory(t, terminated)
	require.Equal(t, "schema v1 is no longer supported", adv.Reason)
	require.Equal(t, uint64(1), adv.StreamSeq)

	require.Never(t, func() bool { return rec.Count() > 1 }, 3*ackWait, samplingInterval,
		"a terminated message must never be delivered again")
	w.awaitSettled(t, consumer)
}

// MaxDeliver bounds how often a failing message is retried. Once the budget is
// spent the server gives up — exactly MaxDeliver deliveries, never one more —
// and raises the max-deliveries advisory, the hook a dead-letter flow hangs
// off: the package has no dead-letter queue of its own, so this advisory is
// the whole terminal signal.
func TestDelivery_MaxDeliverEndsRedeliveryWithAnAdvisory(t *testing.T) {
	t.Parallel()

	const maxDeliver = 3

	w := newWorld(t)
	consumer := uniqueName(t)
	exhausted := watchAdvisory(t, w.provider.NATSConn(), "MAX_DELIVERIES", w.name, consumer)

	rec := brokerit.NewRecorder(w.subject, func(_ context.Context, m *msg.Message, _ int) { _ = m.Nak() })
	cfg := w.durable(consumer, ackWait, func(c *jetstream.ConsumerConfig) { c.MaxDeliver = maxDeliver })
	subscribe(t, w.provider, natsprovider.SubscriberWithConsumer(cfg), rec)

	w.publish(t, "always-fails")

	adv := awaitAdvisory(t, exhausted)
	require.Equal(t, uint64(1), adv.StreamSeq)
	require.Equal(t, uint64(maxDeliver), adv.Deliveries)

	require.Never(t, func() bool { return rec.Count() > maxDeliver }, 3*ackWait, samplingInterval,
		"the server delivered past MaxDeliver")
	require.Equal(t, maxDeliver, rec.Attempts("always-fails"))
}

// A handler that neither acks nor naks leaves the message to the ack timer.
// The server must redeliver it once the timer expires, not before — and the
// message must have told the handler how long that timer was.
func TestDelivery_UnackedMessageIsRedeliveredAfterTheAckWait(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	consumer := uniqueName(t)

	timeouts := make(chan time.Duration, 4)
	rec := brokerit.NewRecorder(w.subject, func(_ context.Context, m *msg.Message, attempt int) {
		timeouts <- m.AckTimeout
		if attempt > 1 {
			_ = m.Ack()
		}
	})
	subscribe(t, w.provider, natsprovider.SubscriberWithConsumer(w.durable(consumer, ackWait)), rec)

	w.publish(t, "forgotten")
	info := w.awaitSettled(t, consumer)

	got := rec.DeliveriesOf("forgotten")
	require.Len(t, got, 2)
	// The server starts the ack timer when it sends the message, a moment
	// before the handler records it, so allow the gap that delivery latency
	// shaves off.
	require.GreaterOrEqual(t, got[1].At.Sub(got[0].At), ackWait*9/10, "redelivered before the ack wait expired")
	require.Equal(t, uint64(2), info.Delivered.Consumer, "the server must count the redelivery")
	require.Equal(t, ackWait, <-timeouts, "the message must carry the consumer's ack wait")
}

// A handler that takes several ack waits to finish keeps its message by
// sending in-progress heartbeats through the inprogress Manager. The server
// must treat each heartbeat as a fresh ack timer: the message is delivered
// once, acknowledged once, and never redelivered.
func TestDelivery_InProgressHeartbeatsHoldOffRedelivery(t *testing.T) {
	t.Parallel()

	const (
		hold      = 3 * ackWait
		heartbeat = ackWait / 4
	)

	mgr := inprogress.New()
	tickDone := make(chan struct{})
	go func() {
		defer close(tickDone)
		ticker := time.NewTicker(heartbeat / 4)
		defer ticker.Stop()
		for {
			select {
			case <-t.Context().Done():
				return
			case <-ticker.C:
				_ = mgr.RunTickCycle(t.Context())
			}
		}
	}()
	t.Cleanup(func() { <-tickDone })

	w := newWorld(t)
	consumer := uniqueName(t)
	rec := brokerit.NewRecorder(w.subject, func(ctx context.Context, m *msg.Message, _ int) {
		stop := mgr.Register(m, heartbeat)
		defer stop()

		select {
		case <-time.After(hold):
		case <-ctx.Done():
			return
		}
		_ = m.Ack()
	})
	subscribe(t, w.provider, natsprovider.SubscriberWithConsumer(w.durable(consumer, ackWait)), rec)

	w.publish(t, "slow")
	info := w.awaitSettled(t, consumer)
	require.Equal(t, uint64(1), info.Delivered.Consumer, "a heartbeating handler lost its message to the ack timer")

	require.Never(t, func() bool { return rec.Count() > 1 }, 2*ackWait, samplingInterval,
		"the message came back after it was acknowledged")
}

// A handler panic is recovered by the subscriber: the message is left
// unacknowledged and redelivered once the ack wait expires, and the
// subscription keeps consuming the messages behind it.
func TestDelivery_PanicIsRedeliveredAndTheSubscriptionSurvives(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	consumer := uniqueName(t)
	rec := brokerit.NewRecorder(w.subject, func(_ context.Context, m *msg.Message, attempt int) {
		if string(m.Data) == "explodes-once" && attempt == 1 {
			panic("handler bug")
		}
		_ = m.Ack()
	})
	subscribe(t, w.provider, natsprovider.SubscriberWithConsumer(w.durable(consumer, ackWait)), rec)

	w.publish(t, "explodes-once", "behind-it")
	w.awaitSettled(t, consumer)

	require.Equal(t, 2, rec.Attempts("explodes-once"), "a panicked delivery must be redelivered")
	require.Equal(t, 1, rec.Attempts("behind-it"), "the subscription must survive the panic")
}
