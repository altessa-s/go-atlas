// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package idempotency provides deduplication middleware for broker subscribers.
//
// It wraps a [broker.SubscriberHandler] with duplicate detection backed by [idempotency.Keeper],
// offering effectively-once message processing semantics on top of at-least-once broker delivery.
//
// # Architecture
//
// The middleware intercepts message acknowledgment through a decorated [msg.Acker].
// Before invoking the underlying subscriber handler, it acquires a distributed lock for the
// message's idempotency key:
//
//   - If the key is already marked as SUCCESS: positive acknowledgment (Ack) is sent immediately, skipping the handler.
//   - If the key is IN_PROGRESS: negative acknowledgment with backoff (NakWithBackOff) is sent, allowing retry without losing the message.
//   - If the lock is acquired: the handler is invoked with a decorated message acker that automatically calls [idempotency.Keeper.Complete]
//     on Ack, or [idempotency.Keeper.Release] on Nak, Term, or handler panics.
//
// # Key Extraction
//
// Idempotency keys are extracted via [KeyFunc]. By default, [DefaultKeyFunc] inspects message metadata
// ([msg.MetaKeyDeduplicateId] followed by [msg.MetaKeyMessageId]).
// For protobuf payloads whose business identifier resides in the payload itself, [ProtoKeyFunc] unmarshals
// the message and reads the specified field.
//
// # Usage
//
// Wrap an existing subscriber handler:
//
//	h := brokeridem.Wrap(myHandler, keeper, "email_sender",
//	    brokeridem.WithKeyFunc(brokeridem.DefaultKeyFunc),
//	)
//	if err := subscriber.Subscribe(ctx, h); err != nil {
//	    log.Fatal(err)
//	}
package idempotency
