// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/transport/broker/msg"

	natsprovider "github.com/altessa-s/go-atlas/transport/broker/providers/nats"
)

// funcHandler adapts a function to broker.SubscriberHandler.
type funcHandler struct {
	topic  string
	handle func(context.Context, *msg.Message)
}

func (h funcHandler) Handle(ctx context.Context, m *msg.Message) { h.handle(ctx, m) }
func (h funcHandler) Topic() string                              { return h.topic }

// TestSubscriber_Metrics covers subscriber metric labels and failure counting
// against an embedded JetStream server: a wildcard subscription is labeled by
// its subscription subject, and Nak, Term (also when issued asynchronously after
// the handler returned) and handler panics each count as one processing error.
func TestSubscriber_Metrics(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	conn, js := testhelpers.ConnectJetStream(t, ns)
	_, err := js.CreateStream(t.Context(), jetstream.StreamConfig{Name: "ORDERS", Subjects: []string{"orders.>"}})
	require.NoError(t, err)

	tc := testhelpers.NewTestCollector()
	provider, err := natsprovider.New(conn, natsprovider.WithCollector(tc))
	require.NoError(t, err)

	const topic = "orders.*.created"
	done := make(chan struct{}, 8)
	handler := funcHandler{topic: topic, handle: func(_ context.Context, m *msg.Message) {
		if string(m.Data) == "async-nak" {
			// Rejected by a worker after the handler has returned.
			go func() {
				_ = m.Nak(time.Hour)
				done <- struct{}{}
			}()
			return
		}
		defer func() { done <- struct{}{} }()
		switch string(m.Data) {
		case "nak":
			_ = m.Nak(time.Hour) // delay keeps the message from being redelivered during the test
		case "term":
			_ = m.Term()
		case "panic":
			panic("handler failure")
		default:
			_ = m.Ack()
		}
	}}

	sub := natsprovider.SubscriberWithConsumer(&jetstream.ConsumerConfig{
		Durable:       "metrics-test",
		FilterSubject: topic,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})(provider)
	require.NoError(t, sub.Subscribe(t.Context(), handler))
	t.Cleanup(sub.Unsubscribe)

	payloads := map[string]string{
		"orders.t1.created": "ok",
		"orders.t2.created": "ok",
		"orders.t3.created": "nak",
		"orders.t4.created": "term",
		"orders.t5.created": "panic",
		"orders.t6.created": "async-nak",
	}
	for subject, payload := range payloads {
		_, err := js.Publish(t.Context(), subject, []byte(payload))
		require.NoError(t, err)
	}
	for range payloads {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			require.FailNow(t, "handler was not invoked for every message")
		}
	}

	counter := func(name, subject string) float64 {
		return testhelpers.GetCounterValue(t, tc, name, "subject", subject)
	}
	// The panic counter increments in a deferred recovery after done is sent.
	require.Eventually(t, func() bool {
		return counter("test_broker_message_processing_errors_total", topic) == 4
	}, 5*time.Second, 10*time.Millisecond)
	require.Equal(t, float64(len(payloads)), counter("test_broker_messages_received_total", topic))
	require.Zero(t, counter("test_broker_messages_received_total", "orders.t1.created"))
}

// TestSubscriber_EphemeralConsumerFiltersBySubject verifies that an ephemeral
// subscription only receives messages for its own subject, not the rest of the
// stream it shares, and only those published after it subscribed.
func TestSubscriber_EphemeralConsumerFiltersBySubject(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	conn, js := testhelpers.ConnectJetStream(t, ns)
	_, err := js.CreateStream(t.Context(), jetstream.StreamConfig{Name: "ORDERS", Subjects: []string{"orders.>"}})
	require.NoError(t, err)

	provider, err := natsprovider.New(conn)
	require.NoError(t, err)

	// Already in the stream before Subscribe: must not be delivered.
	_, err = js.Publish(t.Context(), "orders.0.created", []byte("x"))
	require.NoError(t, err)

	got := make(chan string, 8)
	sub := natsprovider.SubscriberWithEphemeralConsumer()(provider)
	require.NoError(t, sub.Subscribe(t.Context(), funcHandler{topic: "orders.*.created", handle: func(_ context.Context, m *msg.Message) {
		_ = m.Ack()
		got <- m.Topic
	}}))
	t.Cleanup(sub.Unsubscribe)

	for _, subject := range []string{"orders.1.deleted", "orders.1.created", "orders.2.updated"} {
		_, err := js.Publish(t.Context(), subject, []byte("x"))
		require.NoError(t, err)
	}

	select {
	case subject := <-got:
		require.Equal(t, "orders.1.created", subject)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "matching message was not delivered")
	}
	select {
	case subject := <-got:
		require.FailNow(t, "unexpected delivery", subject)
	case <-time.After(300 * time.Millisecond):
	}
}
