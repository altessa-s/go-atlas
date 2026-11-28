// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package broker

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	"github.com/altessa-s/go-atlas/messaging/broker/internal/traceheaders"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/propagation"
)

// ErrPublishConverterNotSet is returned by [Broker.PublishAny] when no
// [PublishConverter] has been configured via [WithPublishConverter].
var ErrPublishConverterNotSet = errors.New("publish converter not set")

// ErrConnectionClosed reports a publish that failed because the provider's
// connection is closed for good and will not reconnect. Providers wrap their
// driver's closed-connection error with it, so callers classify the failure
// without importing the driver; the broker outbox treats it as permanent and
// dead-letters the event instead of retrying.
var ErrConnectionClosed = errors.New("broker connection closed")

// Broker provides a unified interface for message publishing and subscribing.
// It abstracts the underlying message transport ([Provider]) and enhances
// reliability through an [Outboxer]. Create instances using [New] with
// optional configuration [Option] values.
//
// All exported methods are safe for concurrent use.
type Broker struct {
	tracer           tracing.Tracer
	messagingSystem  string
	consumerName     string
	provider         Provider     // Underlying message broker provider.
	logger           *slog.Logger // Logger instance for logging within the broker.
	outbox           Outboxer     // Outbox instance for reliable message publishing.
	publishConverter PublishConverter
	metrics          *brokerMetrics
}

// New creates a new Broker instance with the given Provider and options.
// The Provider defines the actual messaging system (e.g., NATS, Kafka).
// If no Outbox is configured, messages are published directly via the Provider.
//
// Example:
//
//	provider, _ := nats.New(conn)
//	b := broker.New(provider, broker.WithLogger(logger))
func New(provider Provider, opt ...Option) *Broker {
	cfg := newOptions(opt...)

	outbox := cfg.outbox
	if outbox == nil {
		outbox = &nopOutbox{provider: provider}
	}

	system := cfg.messagingSystem
	if system == "" && cfg.tracer != nil {
		if named, ok := provider.(interface{ MessagingSystem() string }); ok {
			system = named.MessagingSystem()
		}
	}

	return &Broker{
		tracer:           cfg.tracer,
		messagingSystem:  system,
		consumerName:     cfg.consumerName,
		provider:         provider,
		logger:           cfg.logger,
		outbox:           outbox,
		publishConverter: cfg.publishConverter,
		metrics:          newBrokerMetrics(cfg.meter, cfg.subjectLabelLimit),
	}
}

// Publish sends a single message using the configured Outboxer.
// The Outboxer handles delivery specifics, potentially including persistence for reliability.
//
// Example:
//
//	err := b.Publish(ctx, msg.Message{Topic: "events", Data: payload})
func (b *Broker) Publish(ctx context.Context, msg msg.Message) error {
	stop := b.metrics.publishDuration.Start()
	msg.Metadata = traceheaders.Inject(msg.Metadata, propagation.TraceContextHeaders(ctx))
	err := b.outbox.Publish(ctx, msg)
	stop()
	if err != nil {
		b.recordPublishError(err, msg)
		return err
	}
	b.metrics.publishedFor(msg.Topic).Inc()
	return nil
}

// PublishBatch sends multiple messages using the configured Outboxer.
// This can be more efficient than publishing individual messages.
//
// Example:
//
//	err := b.PublishBatch(ctx, msg1, msg2, msg3)
func (b *Broker) PublishBatch(ctx context.Context, msgs ...msg.Message) error {
	if headers := propagation.TraceContextHeaders(ctx); headers != nil {
		msgs = slices.Clone(msgs)
		for i := range msgs {
			msgs[i].Metadata = traceheaders.Inject(msgs[i].Metadata, headers)
		}
	}
	stop := b.metrics.publishDuration.Start()
	err := b.outbox.PublishBatch(ctx, msgs...)
	stop()
	if err != nil {
		b.recordPublishError(err, msgs...)
		return err
	}
	for _, m := range msgs {
		b.metrics.publishedFor(m.Topic).Inc()
	}
	return nil
}

// PublishAny converts arbitrary types to messages and publishes them via the Outboxer.
// Requires a PublishConverter configured via WithPublishConverter; returns ErrPublishConverterNotSet otherwise.
// If conversion fails for any message, the entire operation is aborted.
//
// Example:
//
//	err := b.PublishAny(ctx, userCreatedEvent, orderPlacedEvent)
func (b *Broker) PublishAny(ctx context.Context, m ...any) error {
	if b.publishConverter == nil {
		return ErrPublishConverterNotSet
	}

	var msgs = make([]msg.Message, 0, len(m))
	for _, v := range m {
		convertedMsg, err := b.publishConverter(v)
		if err != nil {
			b.logger.Error("message conversion failed", slog.Any("error", err))
			return err
		}
		msgs = append(msgs, convertedMsg)
	}

	return b.PublishBatch(ctx, msgs...)
}

// Subscriber creates a subscriber with factory and applies the broker's tracing.
// A nil factory uses the provider's DefaultSubscriber. To retain provider-owned
// shutdown for a configured subscriber, call the provider's constructor inside factory.
//
// Example:
//
//	sub := b.Subscriber(nil)
//	sub.Subscribe(ctx, handler)
func (b *Broker) Subscriber(factory SubscriberFactory) Subscriber {
	if factory == nil {
		factory = b.provider.DefaultSubscriber
	}
	sub := factory()
	if b.tracer == nil || sub == nil {
		return sub
	}
	return &tracedSubscriber{Subscriber: sub, broker: b}
}

// recordPublishError records a failed publish attempt for every message in mm:
// the per-subject error counter is incremented and the failure is logged with
// the message subject.
func (b *Broker) recordPublishError(err error, mm ...msg.Message) {
	for _, m := range mm {
		b.metrics.errorsFor(m.Topic).Inc()
		b.logger.Error("publish failed",
			slog.String("subject", m.Topic),
			slog.Any("error", err))
	}
}

// nopOutbox is a no-operation Outboxer that publishes directly via the Provider.
// It is used as the default when no custom Outboxer is configured for the Broker.
type nopOutbox struct {
	provider Provider
}

// Publish calls the provider's Publish method directly without persistence.
func (no *nopOutbox) Publish(ctx context.Context, m msg.Message) error {
	return no.provider.Publish(ctx, m)
}

// PublishBatch calls the provider's PublishBatch method directly without persistence.
func (no *nopOutbox) PublishBatch(ctx context.Context, mm ...msg.Message) error {
	return no.provider.PublishBatch(ctx, mm...)
}
