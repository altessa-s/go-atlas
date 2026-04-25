// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package outbox implements the Transactional Outbox pattern for at-least-once event delivery guarantees.
// It persists events to a Store before dispatching them via a Handler, with background workers
// for dispatch, retry, cleanup, and unlocking.
//
// This package is transport-agnostic. The Handler callback determines how events are delivered
// (message broker, HTTP, gRPC, etc.). For broker-specific integration, see transport/broker/outbox.
//
// Example:
//
//	store, _ := mongostore.New(db)
//	handler := func(ctx context.Context, event outbox.Event) error {
//	    return publishToKafka(ctx, event.Key, event.Payload)
//	}
//	ob := outbox.New(store, handler, outbox.WithLogger(slog.Default()))
//	ob.Save(ctx, outbox.Event{Key: "orders.created", Payload: data})
package outbox
