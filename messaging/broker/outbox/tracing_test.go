// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/messaging/broker"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"
	"github.com/altessa-s/go-atlas/messaging/broker/outbox"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/propagation"
	"github.com/altessa-s/go-atlas/observability/tracing/sampler"

	natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
)

type spanSink struct {
	*adapters.MultiAdapter
	mu    sync.Mutex
	spans []adapters.SpanData
}

func (s *spanSink) ExportSpans(_ context.Context, spans []adapters.SpanData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spans = append(s.spans, spans...)
	return nil
}
func (s *spanSink) recorded() []adapters.SpanData {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.spans)
}

type publishFunc func(context.Context, msg.Message) error

func (f publishFunc) Publish(ctx context.Context, m msg.Message) error { return f(ctx, m) }

type handlerFunc func(context.Context, *msg.Message)

func (h handlerFunc) Topic() string                              { return "events.>" }
func (h handlerFunc) Handle(ctx context.Context, m *msg.Message) { h(ctx, m) }

const sampledParent = "00-11111111111111111111111111111111-2222222222222222-01"
const unsampledParent = "00-11111111111111111111111111111111-2222222222222222-00"

func TestOutboxPublishPropagatesRequestContext(t *testing.T) {
	store := &oneShotStore{}
	pub := &capturingPublisher{}
	ob := outbox.New(store, pub)
	ctx := propagation.NewTraceContext().Extract(t.Context(), propagation.MapCarrier{"traceparent": sampledParent})
	plain := msg.Message{Topic: "events"}
	explicit := msg.Message{Topic: "events", Metadata: msg.Meta{{Key: "traceparent", Value: unsampledParent}}}
	require.NoError(t, ob.Publish(ctx, plain))
	require.NoError(t, ob.PublishBatch(ctx, plain, explicit))
	require.Len(t, store.saved, 3)
	handler := outbox.NewHandler(pub)
	for _, event := range store.saved {
		require.NoError(t, handler(t.Context(), event))
	}
	for i, want := range []string{sampledParent, sampledParent, unsampledParent} {
		require.Equal(t, want, pub.published()[i].Metadata.Map()["traceparent"])
	}
}

func TestPublishOutboxJetStreamConsumerTrace(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	conn, js := natstest.ConnectJetStream(t, natstest.StartNATSServer(t))
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{Name: "EVENTS", Subjects: []string{"events.>"}})
	require.NoError(t, err)
	provider, err := natsprovider.New(conn)
	require.NoError(t, err)
	sink := &spanSink{MultiAdapter: adapters.NewMultiAdapter()}
	tracer := tracing.New(tracing.WithAdapter(sink))
	store := &oneShotStore{}
	ob := outbox.New(store, provider, outbox.WithTracer(tracer), outbox.WithMessagingSystem("nats"))
	b := broker.New(provider, broker.WithOutbox(ob), broker.WithTracer(tracer))
	sub := b.Subscriber(func() broker.Subscriber {
		return provider.Subscriber(natsprovider.SubscriberWithConsumerName("worker"))
	})
	_, err = provider.CreateConsumer(ctx, "EVENTS", &jetstream.ConsumerConfig{Durable: "worker", AckPolicy: jetstream.AckExplicitPolicy})
	require.NoError(t, err)
	type delivery struct {
		meta    msg.Meta
		headers propagation.MapCarrier
		err     error
	}
	got := make(chan delivery, 1)
	require.NoError(t, sub.Subscribe(ctx, handlerFunc(func(ctx context.Context, m *msg.Message) {
		headers := propagation.MapCarrier{}
		propagation.NewTraceContext().Inject(ctx, headers)
		got <- delivery{m.Metadata, headers, m.Term("invalid payload")}
	})))
	t.Cleanup(sub.Unsubscribe)
	request := propagation.NewTraceContext().Extract(ctx, propagation.MapCarrier{"traceparent": sampledParent, "tracestate": "vendor=blue"})
	const topic = "events.updated.123"
	require.NoError(t, b.Publish(request, msg.Message{Topic: topic, Metadata: msg.Meta{
		{Key: msg.MetaKeyDeduplicateId, Value: ""},
	}}))
	require.NoError(t, tracer.ForceFlush(ctx))
	require.Empty(t, sink.recorded())
	require.Len(t, store.saved, 1)
	event := store.saved[0]
	store.fetch = store.saved
	require.NoError(t, ob.RunDispatchCycle(ctx))
	var d delivery
	select {
	case d = <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("consumer did not receive the outbox event")
	}
	require.NoError(t, d.err)
	sub.Unsubscribe()
	require.NoError(t, tracer.Shutdown(t.Context()))
	spans := sink.recorded()
	require.Len(t, spans, 2)
	byName := map[string]adapters.SpanData{}
	for _, span := range spans {
		byName[span.Name] = span
	}
	send, process := byName["outbox send"], byName["process worker"]
	require.Equal(t, adapters.SpanKindProducer, send.Kind)
	require.Equal(t, "2222222222222222", send.ParentID.String())
	require.Equal(t, "11111111111111111111111111111111", send.TraceID.String())
	require.Equal(t, adapters.StatusOK, send.Status)
	require.Equal(t, adapters.SpanKindConsumer, process.Kind)
	require.Equal(t, send.SpanID, process.ParentID)
	require.Equal(t, send.TraceID, process.TraceID)
	require.Equal(t, adapters.StatusError, process.Status)
	for _, span := range spans {
		require.Contains(t, span.Attributes, adapters.Attribute{Key: "messaging.system", Value: "nats"})
		require.Contains(t, span.Attributes, adapters.Attribute{Key: "messaging.destination.name", Value: topic})
		require.Contains(t, span.Attributes, adapters.Attribute{Key: "messaging.message.id", Value: event.Id})
	}
	require.Contains(t, process.Attributes, adapters.Attribute{Key: "messaging.consumer.group.name", Value: "worker"})
	require.Contains(t, send.Attributes, adapters.Attribute{Key: "outbox.attempt", Value: int64(1)})
	require.Equal(t, "00-"+send.TraceID.String()+"-"+send.SpanID.String()+"-01", d.meta.Map()["traceparent"])
	require.Equal(t, "00-"+process.TraceID.String()+"-"+process.SpanID.String()+"-01", d.headers["traceparent"])
	require.Equal(t, "vendor=blue", d.headers["tracestate"])
	require.Equal(t, event.Id, d.meta.Map()[msg.MetaKeyDeduplicateId])
}

func TestDispatchContextWithoutTracer(t *testing.T) {
	poll := propagation.NewTraceContext().Extract(t.Context(), propagation.MapCarrier{"traceparent": sampledParent})
	ctx, cancel := context.WithCancel(poll)
	cancel()
	for _, parent := range []string{"", unsampledParent} {
		var traceMeta msg.Meta
		if parent != "" {
			traceMeta = msg.Meta{{Key: "traceparent", Value: parent}}
		}
		event := storedEvent(t, msg.Message{Topic: "events", Metadata: traceMeta}, "event")
		pub := publishFunc(func(ctx context.Context, _ msg.Message) error {
			headers := propagation.MapCarrier{}
			propagation.NewTraceContext().Inject(ctx, headers)
			require.Equal(t, parent, headers["traceparent"])
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			return nil
		})
		require.NoError(t, outbox.NewHandler(pub)(ctx, event))
	}
}

func TestDispatchParentAndFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, parent   string
		traced, record bool
	}{
		{"sampled", sampledParent, true, true}, {"unsampled", unsampledParent, true, false}, {"absent", "", true, false},
		{"invalid", "bad", true, false}, {"no tracer", sampledParent, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sink := &spanSink{MultiAdapter: adapters.NewMultiAdapter()}
			tracer := tracing.New(tracing.WithAdapter(sink), tracing.WithSampler(sampler.AlwaysOn()))
			// A span already on the poller's context must never become the parent.
			poll := propagation.NewTraceContext().Extract(t.Context(), propagation.MapCarrier{"traceparent": sampledParent})
			meta := msg.Meta{}
			if tc.parent != "" {
				meta = append(meta, msg.MetaData{Key: "traceparent", Value: tc.parent})
			}
			event := storedEvent(t, msg.Message{Topic: "topic", Metadata: meta}, "event")
			event.Attempts, event.CreatedAt = 2, time.Now().Add(time.Minute)
			fail := errors.New("publish to nats://publisher:private@broker.example failed")
			pub := &capturingPublisher{err: fail}
			var opts []outbox.Option
			if tc.traced {
				opts = append(opts, outbox.WithTracer(tracer))
			}
			require.ErrorIs(t, outbox.NewHandler(pub, opts...)(poll, event), fail)
			sent := pub.published()[0].Metadata.Map()["traceparent"]
			require.NoError(t, tracer.Shutdown(t.Context()))
			spans := sink.recorded()
			if !tc.record {
				require.Empty(t, spans)
				require.Equal(t, tc.parent, sent)
				return
			}
			require.Len(t, spans, 1)
			s := spans[0]
			require.NotEqual(t, tc.parent, sent)
			require.Equal(t, adapters.StatusError, s.Status)
			require.Equal(t, "publish to nats://broker.example failed", s.StatusDesc)
			require.Len(t, s.Events, 1)
			require.Equal(t, "exception", s.Events[0].Name)
			var messages []any
			for _, attr := range s.Events[0].Attributes {
				if attr.Key == "exception.message" {
					messages = append(messages, attr.Value)
				}
			}
			require.Equal(t, []any{s.StatusDesc}, messages)
			require.ElementsMatch(t, []adapters.Attribute{
				{Key: "messaging.destination.name", Value: "topic"}, {Key: "messaging.message.id", Value: "event"},
				{Key: "outbox.attempt", Value: int64(3)}, {Key: "outbox.queue_wait_ms", Value: int64(0)},
			}, s.Attributes)
		})
	}
}

func TestConsumerSamplingAndAcks(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, parent      string
		sampling          sampler.Sampler
		recording, failed bool
		ack               func(*msg.Message) error
	}{
		{"unsampled", unsampledParent, sampler.AlwaysOn(), false, false, (*msg.Message).Ack},
		{"new root", "", sampler.AlwaysOn(), true, false, (*msg.Message).Ack},
		{"dropped root", "", sampler.AlwaysOff(), false, false, (*msg.Message).Ack},
		{"nak", sampledParent, sampler.AlwaysOn(), true, true, func(m *msg.Message) error { return m.Nak(time.Minute) }},
		{"backoff", sampledParent, sampler.AlwaysOn(), true, true, func(m *msg.Message) error { return m.NakWithBackOff(func(uint64) time.Duration { return time.Minute }) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sink := &spanSink{MultiAdapter: adapters.NewMultiAdapter()}
			tracer := tracing.New(tracing.WithAdapter(sink), tracing.WithSampler(tc.sampling))
			inherited := propagation.NewTraceContext().Extract(t.Context(), propagation.MapCarrier{"traceparent": sampledParent})
			handler := broker.TraceHandler(handlerFunc(func(ctx context.Context, m *msg.Message) {
				headers := propagation.MapCarrier{}
				propagation.NewTraceContext().Inject(ctx, headers)
				require.Equal(t, tc.recording, tracing.SpanFromContext(ctx).IsRecording())
				if tc.parent == unsampledParent {
					require.Equal(t, unsampledParent, headers["traceparent"])
					require.Equal(t, "v=state", headers["tracestate"])
				}
				require.NoError(t, tc.ack(m))
			}), tracer, "worker", "")
			require.Equal(t, "events.>", handler.Topic())
			handler.Handle(inherited, &msg.Message{Topic: "events.created", Metadata: msg.MetaFromMap(map[string]string{"traceparent": tc.parent, "tracestate": "v=state"})})
			require.NoError(t, tracer.Shutdown(t.Context()))
			spans := sink.recorded()
			if !tc.recording {
				require.Empty(t, spans)
				return
			}
			require.Len(t, spans, 1)
			require.Equal(t, tc.parent != "", spans[0].ParentID.IsValid())
			require.Equal(t, tc.failed, spans[0].Status == adapters.StatusError)
		})
	}
}
