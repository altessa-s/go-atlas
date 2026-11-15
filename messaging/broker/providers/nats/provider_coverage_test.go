// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/messaging/broker"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"

	natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
)

func newProvider(t *testing.T, opts ...natsprovider.Option) (*natsprovider.NATS, jetstream.JetStream) {
	t.Helper()
	ns := natstest.StartNATSServer(t)
	conn, js := natstest.ConnectJetStream(t, ns)
	p, err := natsprovider.NewWithContext(t.Context(), conn, opts...)
	require.NoError(t, err)
	return p, js
}

func TestProvider_Accessors(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	conn, _ := natstest.ConnectJetStream(t, ns)
	p, err := natsprovider.New(conn)
	require.NoError(t, err)

	require.Same(t, conn, p.NATSConn())
	require.NotNil(t, p.JetStream())
	require.Empty(t, slices.Collect(p.Subscribers()))

	s1 := p.Subscriber(nil)
	s2 := p.Subscriber(natsprovider.SubscriberWithEphemeralConsumer())
	got := slices.Collect(p.Subscribers())
	require.Equal(t, []broker.Subscriber{s1, s2}, got)

	for range p.Subscribers() {
		break // early stop must not panic
	}

	select {
	case <-s1.(interface{ Closed() <-chan struct{} }).Closed():
	default:
		t.Fatal("a never-subscribed subscriber reports closed")
	}

	p.UnsubscribeAll()
	require.Empty(t, slices.Collect(p.Subscribers()))
}

func TestProvider_StreamAndConsumerManagement(t *testing.T) {
	t.Parallel()

	p, js := newProvider(t)
	ctx := t.Context()

	_, err := p.CreateStream(ctx, nil)
	require.Error(t, err)
	_, err = p.CreateStream(ctx, &jetstream.StreamConfig{})
	require.Error(t, err)
	_, err = p.CreateStream(ctx, &jetstream.StreamConfig{Name: "BAD", Subjects: []string{"a.>"}, MaxMsgs: -5, Retention: 99})
	require.Error(t, err, "server rejects an invalid stream")

	stream, err := p.CreateStream(ctx, &jetstream.StreamConfig{Name: "ORDERS", Subjects: []string{"orders.>"}})
	require.NoError(t, err)
	require.Equal(t, "ORDERS", stream.CachedInfo().Config.Name)

	_, err = p.CreateStreams(ctx)
	require.Error(t, err)
	_, err = p.CreateStreams(ctx, nil)
	require.Error(t, err)
	_, err = p.CreateStreams(ctx, &jetstream.StreamConfig{})
	require.Error(t, err)
	_, err = p.CreateStreams(ctx, &jetstream.StreamConfig{Name: "X", Subjects: []string{"orders.>"}})
	require.Error(t, err, "overlapping subjects are rejected by the server")

	streams, err := p.CreateStreams(ctx,
		&jetstream.StreamConfig{Name: "USERS", Subjects: []string{"users.>"}},
		&jetstream.StreamConfig{Name: "BILLING", Subjects: []string{"billing.>"}},
	)
	require.NoError(t, err)
	require.Len(t, streams, 2)
	require.Contains(t, streams, "USERS")
	require.Contains(t, streams, "BILLING")

	_, err = p.CreateConsumer(ctx, "", &jetstream.ConsumerConfig{})
	require.Error(t, err)
	_, err = p.CreateConsumer(ctx, "ORDERS", nil)
	require.Error(t, err)
	_, err = p.CreateConsumer(ctx, "MISSING", &jetstream.ConsumerConfig{Durable: "c"})
	require.Error(t, err)

	consumer, err := p.CreateConsumer(ctx, "ORDERS", &jetstream.ConsumerConfig{Durable: "workers", AckPolicy: jetstream.AckExplicitPolicy})
	require.NoError(t, err)
	require.Equal(t, "workers", consumer.CachedInfo().Name)

	require.Error(t, p.DeleteConsumer(ctx, "", "workers"))
	require.Error(t, p.DeleteConsumer(ctx, "ORDERS", ""))
	require.Error(t, p.DeleteConsumer(ctx, "ORDERS", "nope"))
	require.NoError(t, p.DeleteConsumer(ctx, "ORDERS", "workers"))
	_, err = js.Consumer(ctx, "ORDERS", "workers")
	require.ErrorIs(t, err, jetstream.ErrConsumerNotFound)

	require.Error(t, p.DeleteStream(ctx, ""))
	require.Error(t, p.DeleteStream(ctx, "MISSING"))
	require.NoError(t, p.DeleteStream(ctx, "USERS"))
	_, err = js.Stream(ctx, "USERS")
	require.ErrorIs(t, err, jetstream.ErrStreamNotFound)
}

func TestProvider_PublishBatch(t *testing.T) {
	t.Parallel()

	p, js := newProvider(t, natsprovider.WithAllowedSubjects("orders.>"))
	ctx := t.Context()
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{Name: "ORDERS", Subjects: []string{"orders.>"}})
	require.NoError(t, err)

	require.NoError(t, p.PublishBatch(ctx))
	require.NoError(t, p.PublishBatch(ctx, *msg.NewMessage("orders.one", []byte("1"))))
	require.NoError(t, p.PublishBatch(ctx,
		*msg.NewMessage("orders.a", []byte("a")),
		*msg.NewMessage("orders.b", []byte("b")),
		*msg.NewMessage("orders.c", []byte("c")),
	))

	err = p.PublishBatch(ctx,
		*msg.NewMessage("orders.a", []byte("a")),
		*msg.NewMessage("users.a", []byte("x")),
	)
	require.ErrorIs(t, err, natsprovider.ErrSubjectNotAllowed)

	stream, err := js.Stream(ctx, "ORDERS")
	require.NoError(t, err)
	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(4), info.State.Msgs, "rejected batch publishes nothing")

	// Subjects no stream captures fail per future and are reported together.
	p2, _ := newProvider(t)
	err = p2.PublishBatch(ctx,
		*msg.NewMessage("unrouted.a", []byte("a")),
		*msg.NewMessage("unrouted.b", []byte("b")),
	)
	require.ErrorIs(t, err, jetstream.ErrNoStreamResponse)
}

func TestSubscriberWithConsumerName_InProgressAndAck(t *testing.T) {
	t.Parallel()

	p, js := newProvider(t)
	ctx := t.Context()
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{Name: "JOBS", Subjects: []string{"jobs.>"}})
	require.NoError(t, err)
	_, err = p.CreateConsumer(ctx, "JOBS", &jetstream.ConsumerConfig{
		Durable: "runner", FilterSubject: "jobs.run", AckPolicy: jetstream.AckExplicitPolicy,
	})
	require.NoError(t, err)

	type result struct {
		data          string
		inProgressErr error
		ackErr        error
	}
	got := make(chan result, 1)
	sub := p.Subscriber(natsprovider.SubscriberWithConsumerName("runner"))
	require.NoError(t, sub.Subscribe(ctx, funcHandler{topic: "jobs.run", handle: func(_ context.Context, m *msg.Message) {
		ip := m.InProgress()
		got <- result{data: string(m.Data), inProgressErr: ip, ackErr: m.Ack()}
	}}))
	t.Cleanup(sub.Unsubscribe)

	require.NoError(t, p.Publish(ctx, *msg.NewMessage("jobs.run", []byte("payload"))))

	select {
	case r := <-got:
		require.Equal(t, "payload", r.data)
		require.NoError(t, r.inProgressErr)
		require.NoError(t, r.ackErr)
	case <-time.After(5 * time.Second):
		t.Fatal("message not delivered")
	}

	consumer, err := js.Consumer(ctx, "JOBS", "runner")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		info, ierr := consumer.Info(ctx)
		return ierr == nil && info.NumAckPending == 0 && info.Delivered.Consumer == 1
	}, 5*time.Second, 20*time.Millisecond)
}
