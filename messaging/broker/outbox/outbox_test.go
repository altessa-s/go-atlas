// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/messaging/broker"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"
	"github.com/altessa-s/go-atlas/messaging/broker/outbox"

	natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
	dataoutbox "github.com/altessa-s/go-atlas/reliability/outbox"
	schedtask "github.com/altessa-s/go-horai/task"
)

// oneShotStore serves a single prepared batch and records what the outbox wrote
// back, which is how these tests observe the dispatch verdict.
type oneShotStore struct {
	mu      sync.Mutex
	fetch   []dataoutbox.Event
	served  bool
	saved   []dataoutbox.Event
	updated []dataoutbox.Event
}

func (s *oneShotStore) FetchUnprocessedEvents(context.Context, uint32) ([]dataoutbox.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.served {
		return nil, nil
	}
	s.served = true
	return s.fetch, nil
}
func (s *oneShotStore) DeleteProcessedEvents(context.Context, time.Duration) error { return nil }
func (s *oneShotStore) UnlockStuckEvents(context.Context, time.Duration) error     { return nil }
func (s *oneShotStore) SaveEvents(_ context.Context, events ...dataoutbox.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = append(s.saved, events...)
	return nil
}
func (s *oneShotStore) UpdateEvents(_ context.Context, events ...dataoutbox.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updated = append(s.updated, events...)
	return nil
}
func (s *oneShotStore) ExpireEvents(context.Context) (int64, error) { return 0, nil }
func (s *oneShotStore) Stats(context.Context) (dataoutbox.Stats, error) {
	return dataoutbox.Stats{}, nil
}

func (s *oneShotStore) written() []dataoutbox.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]dataoutbox.Event(nil), s.updated...)
}

// capturingPublisher records the messages that reached the broker.
type capturingPublisher struct {
	mu   sync.Mutex
	msgs []msg.Message
	err  error
}

func (p *capturingPublisher) Publish(_ context.Context, m msg.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.msgs = append(p.msgs, m)
	return p.err
}

func (p *capturingPublisher) published() []msg.Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]msg.Message(nil), p.msgs...)
}

// storedEvent produces the event the real save path would persist for m, then
// stamps it with id. Building the envelope through Publish rather than by hand
// keeps these tests honest about the serialization format.
func storedEvent(t *testing.T, m msg.Message, id string) dataoutbox.Event {
	t.Helper()

	sink := &oneShotStore{}
	require.NoError(t, outbox.New(sink, &capturingPublisher{}).Publish(t.Context(), m))

	sink.mu.Lock()
	defer sink.mu.Unlock()
	require.Len(t, sink.saved, 1)

	ev := sink.saved[0]
	ev.Id = id
	ev.Status = dataoutbox.StatusInProgress
	return ev
}

// The outbox publishes at-least-once, so the broker needs a stable identity to
// collapse repeats. Without a default, every republish after a timeout looks
// like a brand-new message to JetStream and the duplicate reaches consumers.
func TestDispatch_DefaultsDeduplicationIdToEventId(t *testing.T) {
	t.Parallel()

	pub := &capturingPublisher{}
	store := &oneShotStore{fetch: []dataoutbox.Event{
		storedEvent(t, *msg.NewMessage("billing.invoice.paid", []byte(`{"id":1}`)), "event-id-42"),
	}}

	require.NoError(t, outbox.New(store, pub).RunDispatchCycle(t.Context()))

	published := pub.published()
	require.Len(t, published, 1)

	did, found := published[0].Metadata.Value(msg.MetaKeyDeduplicateId)
	require.True(t, found, "a published message must carry a deduplication ID")
	require.Equal(t, "event-id-42", did,
		"the outbox event ID is stable across retries, so it is the right default")
}

// A caller-supplied ID is usually derived from the business entity and dedupes
// across producers, not just across one event's retries — it must win.
func TestDispatch_KeepsCallerSuppliedDeduplicationId(t *testing.T) {
	t.Parallel()

	m := msg.NewMessageWithMeta("billing.invoice.paid", []byte(`{"id":1}`),
		msg.Meta{{Key: msg.MetaKeyDeduplicateId, Value: "invoice-777"}})

	pub := &capturingPublisher{}
	store := &oneShotStore{fetch: []dataoutbox.Event{storedEvent(t, *m, "event-id-42")}}

	require.NoError(t, outbox.New(store, pub).RunDispatchCycle(t.Context()))

	published := pub.published()
	require.Len(t, published, 1)

	did, found := published[0].Metadata.Value(msg.MetaKeyDeduplicateId)
	require.True(t, found)
	require.Equal(t, "invoice-777", did)
}

func TestDispatch_PreservesMessageTTL(t *testing.T) {
	t.Parallel()
	for _, ttl := range []time.Duration{0, time.Minute, -1} {
		event := storedEvent(t, msg.Message{Topic: "events", TTL: ttl}, "event")
		pub := &capturingPublisher{}
		require.NoError(t, outbox.NewHandler(pub)(t.Context(), event))
		require.Equal(t, ttl, pub.published()[0].TTL)
	}
}

// A stored payload this adapter cannot decode will not decode next time either.
// Retrying it only delays the operator seeing it.
func TestDispatch_RejectsUndecodablePayloadWithoutRetrying(t *testing.T) {
	t.Parallel()

	pub := &capturingPublisher{}
	store := &oneShotStore{fetch: []dataoutbox.Event{{
		Id:        "corrupt-1",
		Key:       "billing.invoice.paid",
		Payload:   []byte("not json"),
		Status:    dataoutbox.StatusInProgress,
		CreatedAt: time.Now().UTC(),
	}}}

	require.NoError(t, outbox.New(store, pub).RunDispatchCycle(t.Context()))

	require.Empty(t, pub.published(), "an undecodable event must never reach the broker")

	written := store.written()
	require.Len(t, written, 1)
	require.Equal(t, dataoutbox.StatusRejected, written[0].Status,
		"a permanently undeliverable event must be dead-lettered, not retried")
}

// capturingRegistrar records the scheduler tasks the adapter registers.
type capturingRegistrar struct {
	mu         sync.Mutex
	registered []schedtask.Config
}

func (r *capturingRegistrar) Register(_ context.Context, cfg schedtask.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registered = append(r.registered, cfg)
	return nil
}

func (r *capturingRegistrar) ids() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.registered))
	for _, c := range r.registered {
		out = append(out, c.ID)
	}
	return out
}

// The adapter passes reliability/outbox options through to the generic outbox. These
// two assertions pick the options whose effect is observable from outside and
// pin that they reach it.
func TestOptions_PayloadLimitReachesTheOutbox(t *testing.T) {
	t.Parallel()

	ob := outbox.New(&oneShotStore{}, &capturingPublisher{}, dataoutbox.WithMaxPayloadBytes(32))

	err := ob.Publish(t.Context(), *msg.NewMessage("billing.invoice.paid", bytes.Repeat([]byte("x"), 64)))
	require.ErrorIs(t, err, dataoutbox.ErrPayloadTooLarge)
}

// Without the stats cycle the backlog and dead-letter gauges stay at zero, which
// reads exactly like a healthy idle outbox — so its registration is the part
// worth pinning.
func TestOptions_SchedulesReachTheOutbox(t *testing.T) {
	t.Parallel()

	reg := &capturingRegistrar{}
	ob := outbox.New(&oneShotStore{}, &capturingPublisher{},
		dataoutbox.WithScheduler(reg),
		dataoutbox.WithDispatchSchedule("@every 2s"),
		dataoutbox.WithUnlockSchedule("@every 11s"),
		dataoutbox.WithStatsSchedule("@every 30s"),
		dataoutbox.WithDispatchTaskID("svc-a-dispatch"),
		dataoutbox.WithStatsTaskID("svc-a-stats"),
	)

	require.NoError(t, ob.RegisterTasks(t.Context()))
	ids := reg.ids()
	require.Contains(t, ids, "svc-a-dispatch", "the dispatch task ID override must reach the scheduler")
	require.Contains(t, ids, "svc-a-stats", "the stats cycle must be registered, under its override")
	require.Len(t, ids, 3, "only the three configured schedules may be registered")
}

// A closed broker connection never reopens, so retrying only burns the budget:
// the adapter classifies it as permanent and the event is dead-lettered at once.
func TestDispatch_ClosedConnectionIsRejected(t *testing.T) {
	t.Parallel()

	pub := &capturingPublisher{err: broker.ErrConnectionClosed}
	store := &oneShotStore{fetch: []dataoutbox.Event{
		storedEvent(t, *msg.NewMessage("billing.invoice.paid", []byte(`{"id":1}`)), "event-id-42"),
	}}

	require.NoError(t, outbox.New(store, pub).RunDispatchCycle(t.Context()))

	written := store.written()
	require.Len(t, written, 1)
	require.Equal(t, dataoutbox.StatusRejected, written[0].Status)
}

// The broker classification is applied after the caller's options, so a
// caller-supplied retry predicate cannot make a closed connection retryable.
func TestNew_ShouldRetryOptionIsOverridden(t *testing.T) {
	t.Parallel()

	pub := &capturingPublisher{err: broker.ErrConnectionClosed}
	store := &oneShotStore{fetch: []dataoutbox.Event{
		storedEvent(t, *msg.NewMessage("billing.invoice.paid", []byte(`{"id":1}`)), "event-id-42"),
	}}

	ob := outbox.New(store, pub, dataoutbox.WithShouldRetry(func(error) bool { return true }))
	require.NoError(t, ob.RunDispatchCycle(t.Context()))

	written := store.written()
	require.Len(t, written, 1)
	require.Equal(t, dataoutbox.StatusRejected, written[0].Status)
}

func TestShouldRetry(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "transient", err: errors.New("broker unavailable"), want: true},
		{name: "undeliverable", err: errors.Join(outbox.ErrUndeliverable, errors.New("bad json")), want: false},
		{name: "closed_connection", err: broker.ErrConnectionClosed, want: false},
		{name: "wrapped_closed_connection", err: fmt.Errorf("publish: %w", broker.ErrConnectionClosed), want: false},
		{name: "raw_nats_closed_connection", err: nats.ErrConnectionClosed, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, outbox.ShouldRetry(tc.err))
		})
	}
}

// A transient publish failure keeps the event retryable, with a backoff.
func TestDispatch_TransientPublishFailureStaysRetryable(t *testing.T) {
	t.Parallel()

	pub := &capturingPublisher{err: errors.New("broker unavailable")}
	store := &oneShotStore{fetch: []dataoutbox.Event{
		storedEvent(t, *msg.NewMessage("billing.invoice.paid", []byte(`{"id":1}`)), "event-id-42"),
	}}

	require.NoError(t, outbox.New(store, pub).RunDispatchCycle(t.Context()))

	written := store.written()
	require.Len(t, written, 1)
	require.Equal(t, dataoutbox.StatusFailed, written[0].Status)
	require.Positive(t, written[0].RetryAfter, "a transient failure must schedule a retry")
}

// A NATS user keeps the closed-connection classification end to end: the
// provider's publish error on a closed connection dead-letters the event.
func TestDispatch_NATSProviderClosedConnectionIsRejected(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	conn, _ := natstest.ConnectJetStream(t, ns)
	provider, err := natsprovider.New(conn)
	require.NoError(t, err)
	conn.Close()

	store := &oneShotStore{fetch: []dataoutbox.Event{
		storedEvent(t, *msg.NewMessage("billing.invoice.paid", []byte(`{"id":1}`)), "event-id-42"),
	}}

	require.NoError(t, outbox.New(store, provider).RunDispatchCycle(t.Context()))

	written := store.written()
	require.Len(t, written, 1)
	require.Equal(t, dataoutbox.StatusRejected, written[0].Status)
}
