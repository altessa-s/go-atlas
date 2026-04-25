// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package outbox provides a broker-specific adapter for the generic transactional outbox pattern.
// It wraps data/outbox with msg.Message conversion and NATS-specific retry logic,
// preserving backward compatibility with the broker.Outboxer interface.
//
// For the generic, transport-agnostic implementation, see data/outbox.
//
// Example:
//
//	store, _ := mongostore.New(db)
//	ob := outbox.New(store, publisher, outbox.WithLogger(slog.Default()))
//	ob.Publish(ctx, msg.Message{Topic: "events", Data: data})
package outbox
