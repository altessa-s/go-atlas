// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package broker_test

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/transport/broker"
	"github.com/altessa-s/go-atlas/transport/broker/msg"
)

var errPublish = errors.New("publish failed")

// stubSubscriber is a minimal broker.Subscriber returned by test factories.
type stubSubscriber struct{}

func (*stubSubscriber) Subscribe(context.Context, broker.SubscriberHandler) error { return nil }
func (*stubSubscriber) Unsubscribe()                                              {}
func (*stubSubscriber) Closed() <-chan struct{}                                   { return nil }

// recordingProvider implements broker.Provider, recording published messages
// and failing on demand.
type recordingProvider struct {
	err error

	mu        sync.Mutex
	published []msg.Message
}

func (p *recordingProvider) Publish(_ context.Context, m msg.Message) error {
	return p.record(m)
}

func (p *recordingProvider) PublishBatch(_ context.Context, mm ...msg.Message) error {
	return p.record(mm...)
}

func (p *recordingProvider) Subscriber(factory broker.SubscriberFactory) broker.Subscriber {
	return factory(p)
}

func (p *recordingProvider) record(mm ...msg.Message) error {
	if p.err != nil {
		return p.err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.published = append(p.published, mm...)
	return nil
}

func (p *recordingProvider) messages() []msg.Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.published)
}

// recordingOutbox implements broker.Outboxer, recording published messages.
type recordingOutbox struct {
	mu        sync.Mutex
	published []msg.Message
}

func (o *recordingOutbox) Publish(_ context.Context, m msg.Message) error {
	return o.record(m)
}

func (o *recordingOutbox) PublishBatch(_ context.Context, mm ...msg.Message) error {
	return o.record(mm...)
}

func (o *recordingOutbox) record(mm ...msg.Message) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.published = append(o.published, mm...)
	return nil
}

func (o *recordingOutbox) messages() []msg.Message {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.published)
}

// capturingHandler is a slog.Handler that records every log record for assertions.
type capturingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *capturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(string) slog.Handler      { return h }

func (h *capturingHandler) snapshot() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.records)
}

// attrValue returns the string form of the named attribute of r, if present.
func attrValue(r slog.Record, key string) (string, bool) {
	var val string
	var found bool
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			val = a.Value.String()
			found = true
			return false
		}
		return true
	})
	return val, found
}

func topics(mm []msg.Message) []string {
	if len(mm) == 0 {
		return nil
	}
	out := make([]string, len(mm))
	for i, m := range mm {
		out[i] = m.Topic
	}
	return out
}

func TestNew_NoOutbox_PublishesViaProvider(t *testing.T) {
	t.Parallel()

	provider := &recordingProvider{}
	b := broker.New(provider)

	require.NoError(t, b.Publish(t.Context(), msg.Message{Topic: "orders", Data: []byte("one")}))
	require.NoError(t, b.PublishBatch(t.Context(),
		msg.Message{Topic: "orders", Data: []byte("two")},
		msg.Message{Topic: "events", Data: []byte("three")},
	))

	require.Equal(t, []string{"orders", "orders", "events"}, topics(provider.messages()))
}

func TestNew_WithOutbox_RoutesThroughOutbox(t *testing.T) {
	t.Parallel()

	provider := &recordingProvider{}
	outbox := &recordingOutbox{}
	b := broker.New(provider, broker.WithOutbox(outbox))

	require.NoError(t, b.Publish(t.Context(), msg.Message{Topic: "orders"}))
	require.NoError(t, b.PublishBatch(t.Context(), msg.Message{Topic: "events"}))

	require.Equal(t, []string{"orders", "events"}, topics(outbox.messages()))
	require.Empty(t, provider.messages(), "provider must not be called when an outbox is configured")
}

func TestPublish_Error(t *testing.T) {
	t.Parallel()

	b := broker.New(&recordingProvider{err: errPublish})

	err := b.Publish(t.Context(), msg.Message{Topic: "orders"})
	require.ErrorIs(t, err, errPublish)
}

func TestNew_RegistersMetrics(t *testing.T) {
	t.Parallel()

	tc := testhelpers.NewTestCollector()
	broker.New(&recordingProvider{}, broker.WithCollector(tc))

	require.True(t, testhelpers.GatherMetric(t, tc, "test_broker_messages_published_total"))
	require.True(t, testhelpers.GatherMetric(t, tc, "test_broker_publish_errors_total"))
	require.True(t, testhelpers.GatherMetric(t, tc, "test_broker_publish_duration_seconds"))
}

func TestPublish_Metrics_Success(t *testing.T) {
	t.Parallel()

	tc := testhelpers.NewTestCollector()
	b := broker.New(&recordingProvider{}, broker.WithCollector(tc))

	require.NoError(t, b.Publish(t.Context(), msg.Message{Topic: "orders"}))

	published := testhelpers.GetCounterValue(t, tc, "test_broker_messages_published_total", "subject", "orders")
	require.Equal(t, float64(1), published)

	failed := testhelpers.GetCounterValue(t, tc, "test_broker_publish_errors_total", "subject", "orders")
	require.Zero(t, failed)

	observations := testhelpers.GetHistogramCount(t, tc, "test_broker_publish_duration_seconds")
	require.Equal(t, uint64(1), observations)
}

func TestPublish_Metrics_Error(t *testing.T) {
	t.Parallel()

	tc := testhelpers.NewTestCollector()
	b := broker.New(&recordingProvider{err: errPublish}, broker.WithCollector(tc))

	require.ErrorIs(t, b.Publish(t.Context(), msg.Message{Topic: "orders"}), errPublish)

	failed := testhelpers.GetCounterValue(t, tc, "test_broker_publish_errors_total", "subject", "orders")
	require.Equal(t, float64(1), failed)

	published := testhelpers.GetCounterValue(t, tc, "test_broker_messages_published_total", "subject", "orders")
	require.Zero(t, published)
}

func TestPublishBatch_Metrics_Success(t *testing.T) {
	t.Parallel()

	tc := testhelpers.NewTestCollector()
	b := broker.New(&recordingProvider{}, broker.WithCollector(tc))

	require.NoError(t, b.PublishBatch(t.Context(),
		msg.Message{Topic: "orders"},
		msg.Message{Topic: "orders"},
		msg.Message{Topic: "events"},
	))

	orders := testhelpers.GetCounterValue(t, tc, "test_broker_messages_published_total", "subject", "orders")
	require.Equal(t, float64(2), orders)

	events := testhelpers.GetCounterValue(t, tc, "test_broker_messages_published_total", "subject", "events")
	require.Equal(t, float64(1), events)

	observations := testhelpers.GetHistogramCount(t, tc, "test_broker_publish_duration_seconds")
	require.Equal(t, uint64(1), observations, "one batch is one duration observation")
}

func TestPublishBatch_Metrics_Error(t *testing.T) {
	t.Parallel()

	tc := testhelpers.NewTestCollector()
	b := broker.New(&recordingProvider{err: errPublish}, broker.WithCollector(tc))

	err := b.PublishBatch(t.Context(),
		msg.Message{Topic: "orders"},
		msg.Message{Topic: "events"},
	)
	require.ErrorIs(t, err, errPublish)

	// Batch failures attribute the error to every affected subject.
	orders := testhelpers.GetCounterValue(t, tc, "test_broker_publish_errors_total", "subject", "orders")
	require.Equal(t, float64(1), orders)

	events := testhelpers.GetCounterValue(t, tc, "test_broker_publish_errors_total", "subject", "events")
	require.Equal(t, float64(1), events)

	unlabeled := testhelpers.GetCounterValue(t, tc, "test_broker_publish_errors_total")
	require.Zero(t, unlabeled)

	published := testhelpers.GetCounterValue(t, tc, "test_broker_messages_published_total", "subject", "orders")
	require.Zero(t, published)
}

func TestPublishAny_NoConverter(t *testing.T) {
	t.Parallel()

	provider := &recordingProvider{}
	b := broker.New(provider)

	err := b.PublishAny(t.Context(), "payload")
	require.ErrorIs(t, err, broker.ErrPublishConverterNotSet)
	require.Empty(t, provider.messages())
}

func TestPublishAny_ConvertsAndPublishes(t *testing.T) {
	t.Parallel()

	provider := &recordingProvider{}
	converter := func(m any) (msg.Message, error) {
		s, ok := m.(string)
		if !ok {
			return msg.Message{}, errors.New("unexpected type")
		}
		return msg.Message{Topic: "events." + s, Data: []byte(s)}, nil
	}

	tc := testhelpers.NewTestCollector()
	b := broker.New(provider,
		broker.WithPublishConverter(converter),
		broker.WithCollector(tc),
	)

	require.NoError(t, b.PublishAny(t.Context(), "created", "deleted"))
	require.Equal(t, []string{"events.created", "events.deleted"}, topics(provider.messages()))

	created := testhelpers.GetCounterValue(t, tc, "test_broker_messages_published_total", "subject", "events.created")
	require.Equal(t, float64(1), created)

	deleted := testhelpers.GetCounterValue(t, tc, "test_broker_messages_published_total", "subject", "events.deleted")
	require.Equal(t, float64(1), deleted)
}

func TestPublishAny_ConversionError(t *testing.T) {
	t.Parallel()

	errConvert := errors.New("conversion failed")
	provider := &recordingProvider{}
	converter := func(m any) (msg.Message, error) {
		if m == "bad" {
			return msg.Message{}, errConvert
		}
		return msg.Message{Topic: "events"}, nil
	}

	tc := testhelpers.NewTestCollector()
	logs := &capturingHandler{}
	b := broker.New(provider,
		broker.WithPublishConverter(converter),
		broker.WithCollector(tc),
		broker.WithLogger(slog.New(logs)),
	)

	err := b.PublishAny(t.Context(), "ok", "bad")
	require.ErrorIs(t, err, errConvert)
	require.Empty(t, provider.messages(), "conversion failure must abort the whole batch")

	// Conversion failures happen before publishing, so no metric is recorded.
	failed := testhelpers.GetCounterValue(t, tc, "test_broker_publish_errors_total")
	require.Zero(t, failed)

	observations := testhelpers.GetHistogramCount(t, tc, "test_broker_publish_duration_seconds")
	require.Zero(t, observations)

	// The conversion failure is still logged at Error level.
	records := logs.snapshot()
	require.Len(t, records, 1)
	require.Equal(t, slog.LevelError, records[0].Level)
}

func TestPublishAny_PublishError(t *testing.T) {
	t.Parallel()

	provider := &recordingProvider{err: errPublish}
	converter := func(any) (msg.Message, error) {
		return msg.Message{Topic: "events"}, nil
	}

	tc := testhelpers.NewTestCollector()
	b := broker.New(provider,
		broker.WithPublishConverter(converter),
		broker.WithCollector(tc),
	)

	require.ErrorIs(t, b.PublishAny(t.Context(), "payload"), errPublish)

	// PublishAny failures attribute the error to the converted message's subject.
	failed := testhelpers.GetCounterValue(t, tc, "test_broker_publish_errors_total", "subject", "events")
	require.Equal(t, float64(1), failed)

	unlabeled := testhelpers.GetCounterValue(t, tc, "test_broker_publish_errors_total")
	require.Zero(t, unlabeled)
}

func TestPublish_Error_LogsSubject(t *testing.T) {
	t.Parallel()

	logs := &capturingHandler{}
	b := broker.New(&recordingProvider{err: errPublish}, broker.WithLogger(slog.New(logs)))

	require.ErrorIs(t, b.Publish(t.Context(), msg.Message{Topic: "orders"}), errPublish)

	records := logs.snapshot()
	require.Len(t, records, 1)
	require.Equal(t, slog.LevelError, records[0].Level)

	subject, ok := attrValue(records[0], "subject")
	require.True(t, ok, "log record must carry the subject attribute")
	require.Equal(t, "orders", subject)

	_, ok = attrValue(records[0], "error")
	require.True(t, ok, "log record must carry the error attribute")
}

func TestSubscriber_DelegatesToProvider(t *testing.T) {
	t.Parallel()

	provider := &recordingProvider{}
	b := broker.New(provider)

	sub := &stubSubscriber{}
	var factoryArg any
	factory := func(p any) broker.Subscriber {
		factoryArg = p
		return sub
	}

	got := b.Subscriber(factory)
	require.Same(t, sub, got)
	require.Same(t, provider, factoryArg)
}
