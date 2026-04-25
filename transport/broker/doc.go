// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package broker provides a high-level message broker abstraction for reliable
// asynchronous communication. It supports multiple messaging systems and
// implements the Transactional Outbox pattern to ensure exactly-once delivery
// semantics in distributed systems.
//
// # Architecture
//
//   - [Broker]: main entry point combining [Publisher] and [Subscriber] creation
//   - [Provider]: transport-specific implementation (e.g. NATS JetStream)
//   - [Outboxer]: transactional outbox for reliable delivery
//   - [msg.Message]: message envelope with topic, payload, and [msg.Acker] acknowledgment
//
// # Usage
//
//	provider, _ := nats.New(conn)
//	b := broker.New(provider, broker.WithOutbox(outboxer))
//
//	// Simple Publish
//	b.Publish(ctx, msg.Message{Topic: "orders.created", Data: orderData})
//
//	// Subscribe with factory
//	sub := b.Subscriber(factory)
//	sub.Subscribe(ctx, handler)
package broker
