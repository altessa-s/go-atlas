// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package broker

import (
	"context"

	"github.com/altessa-s/go-atlas/transport/broker/msg"
)

// SubscriberHandler represents a handler for messages received from a specific topic.
// Implementations encapsulate message processing logic and define which topic they subscribe to.
//
// Handle is called from the subscriber's goroutine; implementations must be
// safe for concurrent invocation when multiple messages arrive simultaneously.
type SubscriberHandler interface {
	// Handle processes an incoming message and manages acknowledgment
	// via [msg.Message.Ack], [msg.Message.Nak], or [msg.Message.Term].
	Handle(ctx context.Context, msg *msg.Message)

	// Topic returns the NATS subject or topic this handler subscribes to.
	Topic() string
}

// Subscriber is the interface for message subscription and lifecycle management.
// It defines methods for subscribing, unsubscribing, and monitoring subscription state.
type Subscriber interface {
	// Subscribe establishes a subscription using the handler's Topic and Handle methods.
	Subscribe(context.Context, SubscriberHandler) error

	// Unsubscribe cancels the subscription and releases associated resources.
	Unsubscribe()

	// Closed returns a channel that closes when the subscriber has fully stopped.
	Closed() <-chan struct{}
}

// Publisher is the interface for sending messages to the broker.
// It provides methods for single, batch, and type-converted message publishing.
// All methods must be safe for concurrent use.
type Publisher interface {
	// Publish sends a single message to the broker.
	Publish(context.Context, msg.Message) error

	// PublishBatch sends multiple messages in a single batch operation.
	PublishBatch(context.Context, ...msg.Message) error

	// PublishAny converts arbitrary types to messages using the configured [PublishConverter].
	// Returns [ErrPublishConverterNotSet] if no converter is configured.
	PublishAny(ctx context.Context, msg ...any) error
}

// SubscriberFactory is a function type that creates a new Subscriber from a provider.
// The provider argument is typed as any to allow flexibility across different implementations.
type SubscriberFactory func(any) Subscriber

// Outboxer defines the interface for the transactional outbox pattern.
// It ensures reliable message delivery by persisting messages before publishing.
type Outboxer interface {
	// Publish persists and sends a single message using the outbox strategy.
	Publish(context.Context, msg.Message) error

	// PublishBatch persists and sends multiple messages using the outbox strategy.
	PublishBatch(context.Context, ...msg.Message) error
}

// Brokerer defines the primary interface for a message broker.
// It combines [Publisher] capabilities with subscriber creation.
type Brokerer interface {
	Publisher
	// Subscriber creates a new [Subscriber] using the provided [SubscriberFactory].
	Subscriber(SubscriberFactory) Subscriber
}

// Provider defines the interface for an underlying message broker provider (e.g., NATS, Kafka).
// It encapsulates provider-specific logic for publishing and subscribing.
type Provider interface {
	// Publish sends a single message using the provider's implementation.
	Publish(context.Context, msg.Message) error

	// PublishBatch sends multiple messages using the provider's implementation.
	PublishBatch(context.Context, ...msg.Message) error

	// Subscriber creates a new Subscriber specific to this provider.
	Subscriber(SubscriberFactory) Subscriber
}

// PublishConverter is a function that converts arbitrary types to msg.Message.
// Implementations handle type assertions, serialization, topic assignment, and metadata.
//
// Example:
//
//	converter := func(m any) (msg.Message, error) {
//		event := m.(MyEvent)
//		data, _ := json.Marshal(event)
//		return msg.Message{Topic: "events", Data: data}, nil
//	}
type PublishConverter func(m any) (msg.Message, error)
