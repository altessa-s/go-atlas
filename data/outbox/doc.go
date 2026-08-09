// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package outbox implements the Transactional Outbox pattern for at-least-once event delivery guarantees.
// It persists events to a Store before dispatching them via a Handler, with background workers
// for dispatch, retry, expiration, cleanup, unlocking, and backlog measurement.
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
//
// # Transactional use
//
// Save must run inside the same store transaction as the business data it describes.
// That atomicity is the whole point: without it, the write can commit while the event
// is lost, or the event can be published for a write that rolled back. With the MongoDB
// store this means passing the session context into Save — see [Outbox.Save].
//
// # Delivery semantics
//
// Delivery is at-least-once, and duplicates are expected rather than exceptional: a handler
// can publish successfully and then fail before the status is written back. Consumers must
// be idempotent. Where the transport supports deduplication, key it off [Event.Id], which is
// assigned once at Save and stays constant across every retry.
//
// Ordering is NOT preserved. A batch is fetched oldest-first but dispatched concurrently,
// and a failed event is rescheduled behind events created after it. Do not rely on the
// order in which the Handler is called, even for a single Key.
//
// # Retries and dead-lettering
//
// Each dispatch cycle spends exactly one attempt per event. A failure is classified by the
// predicate from [WithShouldRetry]: transient failures are rescheduled with exponential
// backoff and jitter, and permanent ones move straight to [StatusRejected]. An event that
// exhausts [WithRetryMaxAttempts] becomes [StatusMaxAttemptReached].
//
// Both terminal states form the dead-letter queue. They are deliberately excluded from
// cleanup, so they accumulate until an operator acts. Schedule the stats cycle and alert on
// the resulting gauges — see [Store.Stats] and [Outbox.RunStatsCycle]; a dead-letter queue
// nobody watches is data loss with extra steps.
package outbox
