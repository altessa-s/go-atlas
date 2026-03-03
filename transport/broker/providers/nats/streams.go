// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/transport/broker"
	"github.com/altessa-s/go-atlas/transport/broker/msg"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ttlNever indicates the message should not expire in JetStream.
const ttlNever = -1

// Publish sends a single message to NATS JetStream.
// Sets NATS headers from metadata including TTL and deduplication ID.
//
// Example:
//
//	err := provider.Publish(ctx, msg.Message{Topic: "events", Data: data})
func (n *Nats) Publish(ctx context.Context, pmsg msg.Message) error {
	if err := n.checkSubjectAllowed(pmsg.Topic); err != nil {
		return err
	}

	// Build nats.Msg directly to avoid the wasted make(Header) inside nats.NewMsg.
	natsMsg := &nats.Msg{
		Subject: pmsg.Topic,
		Data:    pmsg.Data,
		Header:  make(nats.Header, 1+len(pmsg.Metadata)),
	}

	// Set NATS TTL header if TTL is specified in the message.
	if pmsg.TTL != 0 {
		// Nats-TTL header is supported from Nats server v2.11 onwards.
		if pmsg.TTL == ttlNever {
			natsMsg.Header.Set("Nats-TTL", "never") // Special value for no expiry.
		} else {
			natsMsg.Header.Set("Nats-TTL", pmsg.TTL.String())
		}
	}

	if len(pmsg.Metadata) > 0 {
		for _, m := range pmsg.Metadata {
			// Skip NATS-specific headers and the deduplication ID (handled by jetstream.WithMsgID).
			if strings.HasPrefix(m.Key, "Nats-") || m.Key == msg.MetaKeyDeduplicateId {
				continue
			}
			natsMsg.Header.Set(m.Key, m.Value)
		}
	}

	var pubOptions []jetstream.PublishOpt
	if did, found := pmsg.Metadata.Value(msg.MetaKeyDeduplicateId); found && did != "" {
		pubOptions = append(pubOptions, jetstream.WithMsgID(did))
	}

	// Publish the message to JetStream.
	_, err := n.jetStream.PublishMsg(ctx, natsMsg, pubOptions...)
	if err != nil {
		return coreerrs.WrapOperationWithContext(err, "publish message to JetStream", fmt.Sprintf("topic '%s'", pmsg.Topic))
	}
	return nil
}

// PublishBatch sends multiple messages concurrently using the batch processor.
// The operation is not atomic: some messages may be published before the first error is returned.
func (n *Nats) PublishBatch(ctx context.Context, pmsgs ...msg.Message) error {
	return concurrency.Process(ctx, pmsgs, func(ctx context.Context, message msg.Message) error {
		return n.Publish(ctx, message)
	}, concurrency.BatchConfig[msg.Message]{})
}

// Subscriber creates a new JetStream subscriber using the provided factory.
// If factory is nil, uses SubscriberWithEphemeralConsumer.
func (n *Nats) Subscriber(factory broker.SubscriberFactory) broker.Subscriber {
	if factory == nil {
		factory = SubscriberWithEphemeralConsumer()
	}

	s := factory(n) // The factory receives this Nat's instance as the provider argument.

	n.subscribersMx.Lock()
	n.subscribers = append(n.subscribers, s)
	n.subscribersMx.Unlock()

	return s
}

// CreateConsumer creates or updates a JetStream consumer on a stream.
//
// Example:
//
//	consumer, err := provider.CreateConsumer(ctx, "events", &config)
func (n *Nats) CreateConsumer(ctx context.Context, stream string, config *jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	if stream == "" {
		return nil, errors.New("stream name must be provided")
	}
	if config == nil {
		return nil, errors.New("consumer configuration must be provided")
	}
	consumer, err := n.jetStream.CreateOrUpdateConsumer(ctx, stream, *config)
	if err != nil {
		return nil, coreerrs.WrapOperationWithContext(err, fmt.Sprintf("create/update consumer '%s'", config.Name), fmt.Sprintf("stream '%s'", stream))
	}
	return consumer, nil
}

// DeleteConsumer deletes a JetStream consumer from a stream.
func (n *Nats) DeleteConsumer(ctx context.Context, stream, consumerName string) error {
	if stream == "" {
		return errors.New("stream name must be provided")
	}
	if consumerName == "" {
		return errors.New("consumer name must be provided")
	}
	err := n.jetStream.DeleteConsumer(ctx, stream, consumerName)
	if err != nil {
		return coreerrs.WrapOperationWithContext(err, fmt.Sprintf("delete consumer '%s'", consumerName), fmt.Sprintf("stream '%s'", stream))
	}
	return nil
}

// CreateStreams creates or updates multiple JetStream streams.
// Returns a map of stream names to stream instances.
//
// Example:
//
//	streams, err := provider.CreateStreams(ctx, &config1, &config2)
func (n *Nats) CreateStreams(ctx context.Context, configs ...*jetstream.StreamConfig) (map[string]jetstream.Stream, error) {
	if len(configs) == 0 {
		return nil, errors.New("one or more stream configurations must be provided")
	}
	streams := make(map[string]jetstream.Stream, len(configs))
	for _, cfg := range configs {
		if cfg == nil {
			return nil, errors.New("stream configuration cannot be nil")
		}
		if cfg.Name == "" {
			return nil, errors.New("stream name in configuration cannot be empty")
		}
		stream, err := n.jetStream.CreateOrUpdateStream(ctx, *cfg)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, fmt.Sprintf("create/update stream '%s'", cfg.Name))
		}
		streams[cfg.Name] = stream
	}
	return streams, nil
}

// CreateStream creates or updates a single JetStream stream.
//
// Example:
//
//	stream, err := provider.CreateStream(ctx, &jetstream.StreamConfig{Name: "events"})
func (n *Nats) CreateStream(ctx context.Context, cfg *jetstream.StreamConfig) (jetstream.Stream, error) {
	if cfg == nil {
		return nil, errors.New("stream configuration must be provided")
	}
	if cfg.Name == "" {
		return nil, errors.New("stream name in configuration cannot be empty")
	}
	stream, err := n.jetStream.CreateOrUpdateStream(ctx, *cfg)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, fmt.Sprintf("create/update stream '%s'", cfg.Name))
	}
	return stream, nil
}

// DeleteStream deletes a JetStream stream by name.
func (n *Nats) DeleteStream(ctx context.Context, streamName string) error {
	if streamName == "" {
		return errors.New("stream name must be provided")
	}
	err := n.jetStream.DeleteStream(ctx, streamName)
	if err != nil {
		return coreerrs.WrapOperation(err, fmt.Sprintf("delete stream '%s'", streamName))
	}
	return nil
}
