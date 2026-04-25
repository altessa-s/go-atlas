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

	natsMsg, pubOpts := buildNatsMsg(pmsg)

	_, err := n.jetStream.PublishMsg(ctx, natsMsg, pubOpts...)
	if err != nil {
		return coreerrs.WrapOperationWithContext(err, "publish message to JetStream", fmt.Sprintf("topic '%s'", pmsg.Topic))
	}
	return nil
}

// PublishBatch sends multiple messages using JetStream async pipelining.
// All messages are dispatched without waiting for individual ACKs, then
// all futures are collected. This eliminates per-message round-trip latency.
// The operation is not atomic: some messages may be stored before an error is returned.
func (n *Nats) PublishBatch(ctx context.Context, pmsgs ...msg.Message) error {
	if len(pmsgs) == 0 {
		return nil
	}

	// Single message: use synchronous path to avoid async overhead.
	if len(pmsgs) == 1 {
		return n.Publish(ctx, pmsgs[0])
	}

	// Validate all subjects upfront — fail fast before any async publish.
	for i := range pmsgs {
		if err := n.checkSubjectAllowed(pmsgs[i].Topic); err != nil {
			return err
		}
	}

	// Fire all messages asynchronously using JetStream pipelining.
	futures := make([]jetstream.PubAckFuture, 0, len(pmsgs))
	for i := range pmsgs {
		natsMsg, pubOpts := buildNatsMsg(pmsgs[i])

		paf, err := n.jetStream.PublishMsgAsync(natsMsg, pubOpts...)
		if err != nil {
			return coreerrs.WrapOperationWithContext(err, "async publish to JetStream", fmt.Sprintf("topic '%s'", pmsgs[i].Topic))
		}
		futures = append(futures, paf)
	}

	// Collect results — wait for each future individually.
	var errs []error
	for _, paf := range futures {
		select {
		case <-paf.Ok():
			// ACK received.
		case err := <-paf.Err():
			errs = append(errs, err)
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return errors.Join(errs...)
}

// buildNatsMsg converts a broker message into a NATS JetStream message and publish options.
func buildNatsMsg(pmsg msg.Message) (*nats.Msg, []jetstream.PublishOpt) {
	natsMsg := &nats.Msg{
		Subject: pmsg.Topic,
		Data:    pmsg.Data,
		Header:  make(nats.Header, 1+len(pmsg.Metadata)),
	}

	if pmsg.TTL != 0 {
		if pmsg.TTL == ttlNever {
			natsMsg.Header.Set("Nats-TTL", "never")
		} else {
			natsMsg.Header.Set("Nats-TTL", pmsg.TTL.String())
		}
	}

	if len(pmsg.Metadata) > 0 {
		for _, m := range pmsg.Metadata {
			if strings.HasPrefix(m.Key, "Nats-") || m.Key == msg.MetaKeyDeduplicateId {
				continue
			}
			natsMsg.Header.Set(m.Key, m.Value)
		}
	}

	var pubOpts []jetstream.PublishOpt
	if did, found := pmsg.Metadata.Value(msg.MetaKeyDeduplicateId); found && did != "" {
		pubOpts = append(pubOpts, jetstream.WithMsgID(did))
	}

	return natsMsg, pubOpts
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
