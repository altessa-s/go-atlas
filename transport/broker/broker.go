// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package broker

import (
	"context"
	"errors"
	"log/slog"

	"github.com/altessa-s/go-atlas/transport/broker/msg"
)

// ErrPublishConverterNotSet is returned by [Broker.PublishAny] when no
// [PublishConverter] has been configured via [WithPublishConverter].
var ErrPublishConverterNotSet = errors.New("publish converter not set")

// Broker provides a unified interface for message publishing and subscribing.
// It abstracts the underlying message transport ([Provider]) and enhances
// reliability through an [Outboxer]. Create instances using [New] with
// optional configuration [Option] values.
//
// All exported methods are safe for concurrent use.
type Broker struct {
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

	return &Broker{
		provider:         provider,
		logger:           cfg.logger,
		outbox:           outbox,
		publishConverter: cfg.publishConverter,
		metrics:          newBrokerMetrics(cfg.collector),
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
	err := b.outbox.Publish(ctx, msg)
	stop()
	if err != nil {
		b.metrics.publishErrors.Inc()
		return err
	}
	b.metrics.messagesPublished.Inc()
	return nil
}

// PublishBatch sends multiple messages using the configured Outboxer.
// This can be more efficient than publishing individual messages.
//
// Example:
//
//	err := b.PublishBatch(ctx, msg1, msg2, msg3)
func (b *Broker) PublishBatch(ctx context.Context, msgs ...msg.Message) error {
	stop := b.metrics.publishDuration.Start()
	err := b.outbox.PublishBatch(ctx, msgs...)
	stop()
	if err != nil {
		b.metrics.publishErrors.Inc()
		return err
	}
	b.metrics.messagesPublished.Add(float64(len(msgs)))
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
			return err
		}
		msgs = append(msgs, convertedMsg)
	}

	stop := b.metrics.publishDuration.Start()
	err := b.outbox.PublishBatch(ctx, msgs...)
	stop()
	if err != nil {
		b.metrics.publishErrors.Inc()
		return err
	}
	b.metrics.messagesPublished.Add(float64(len(msgs)))
	return nil
}

// Subscriber creates a new message Subscriber using the underlying Provider and factory.
// The factory allows for custom instantiation and configuration of the subscriber.
//
// Example:
//
//	sub := b.Subscriber(nats.SubscriberFactory)
//	sub.Subscribe(ctx, handler)
func (b *Broker) Subscriber(factory SubscriberFactory) Subscriber {
	return b.provider.Subscriber(factory)
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
