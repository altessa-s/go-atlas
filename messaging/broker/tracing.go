// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package broker

import (
	"cmp"
	"context"
	"time"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/propagation"
)

type tracedSubscriber struct {
	Subscriber
	broker *Broker
}

func (s *tracedSubscriber) Subscribe(ctx context.Context, h SubscriberHandler) error {
	name := s.broker.consumerName
	if name == "" {
		if named, ok := s.Subscriber.(interface{ ConsumerName() string }); ok {
			name = named.ConsumerName()
		}
	}
	return s.Subscriber.Subscribe(ctx, TraceHandler(h, s.broker.tracer, cmp.Or(name, "subscriber"), s.broker.messagingSystem))
}

// TraceHandler wraps h in a CONSUMER span named "process <name>". Use a stable
// consumer or durable name. An empty system omits messaging.system. A nil
// tracer returns h unchanged. Broker.Subscriber installs this wrapper when
// WithTracer is set. Pass the wrapped handler to NATS recovery directly.
//
//	handler := broker.TraceHandler(h, tracer, durable, "nats")
//	sub, err := provider.Recovery.Subscribe(ctx, stream, durable, handler, factory)
func TraceHandler(h SubscriberHandler, tracer tracing.Tracer, name, system string) SubscriberHandler {
	if nilcheck.IsNil(tracer) {
		return h
	}
	return &tracedHandler{SubscriberHandler: h, recorder: tracer.Recorder("messaging/broker"), name: name, system: system}
}

type tracedHandler struct {
	SubscriberHandler
	recorder     tracing.Recorder
	name, system string
}

func (h *tracedHandler) Handle(ctx context.Context, m *msg.Message) {
	// The subscription can outlive the request that started it.
	ctx = tracing.ContextWithSpan(ctx, nil)
	if m == nil {
		h.SubscriberHandler.Handle(ctx, m)
		return
	}
	ctx = propagation.NewTraceContext().Extract(ctx, propagation.MapCarrier(m.Metadata.Map()))
	if parent := propagation.RemoteSpanContextFromContext(ctx); parent != nil && !parent.IsSampled() {
		h.SubscriberHandler.Handle(ctx, m)
		return
	}
	attrs := []tracing.Attribute{
		tracing.String("messaging.destination.name", m.Topic),
		tracing.String("messaging.consumer.group.name", h.name),
	}
	if h.system != "" {
		attrs = append(attrs, tracing.String("messaging.system", h.system))
	}
	if id, ok := m.Metadata.Value(msg.MetaKeyDeduplicateId); ok && id != "" {
		attrs = append(attrs, tracing.String("messaging.message.id", id))
	}
	ctx, span := h.recorder.Start(ctx, "process "+h.name, tracing.WithSpanKind(tracing.SpanKindConsumer), tracing.WithAttributes(attrs...))
	defer span.End()
	if span.IsRecording() {
		m = m.CloneWithAcker(&tracedAcker{Acker: m, span: span})
	}
	h.SubscriberHandler.Handle(ctx, m)
}

type tracedAcker struct {
	msg.Acker
	span tracing.Span
}

func (a *tracedAcker) Nak(delay ...time.Duration) error {
	a.span.SetStatus(tracing.StatusError, "nak")
	return a.Acker.Nak(delay...)
}

func (a *tracedAcker) NakWithBackOff(backOff msg.BackOffFunc) error {
	a.span.SetStatus(tracing.StatusError, "nak")
	return a.Acker.NakWithBackOff(backOff)
}

func (a *tracedAcker) Term(reason ...string) error {
	a.span.SetStatus(tracing.StatusError, "term")
	return a.Acker.Term(reason...)
}
