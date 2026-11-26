// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package outbox provides a broker-specific adapter for the generic transactional outbox pattern.
// It wraps reliability/outbox with msg.Message conversion and the broker retry
// classification. [ShouldRetry] marks [ErrUndeliverable] permanent. A closed
// broker connection stops new sends in the current cycle and preserves the
// event's retry budget. The adapter implements the broker.Outboxer interface.
//
// Compaction uses the outbox save time to pick the latest event for a topic
// within a batch. The producer timestamp stays in message metadata.
//
// For the generic, transport-agnostic implementation, see reliability/outbox. Pass its options to [New] through [WithOutboxOptions].
//
// Example:
//
//	store, _ := mongostore.New(db)
//	ob := outbox.New(store, publisher, outbox.WithOutboxOptions(dataoutbox.WithLogger(slog.Default())))
//	ob.Publish(ctx, msg.Message{Topic: "events", Data: data})
package outbox
