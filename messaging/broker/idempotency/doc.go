// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package idempotency provides deduplication middleware for broker subscribers.
//
// It wraps a [broker.SubscriberHandler] with duplicate detection backed by a [Keeper],
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
//   - If the lock is acquired: the handler is invoked with a decorated message acker that automatically calls [Keeper.Complete]
//     on Ack, or [Keeper.Release] on Nak, Term, or handler panics.
//
// # Key Extraction
//
// Idempotency keys are extracted via [KeyFunc]. By default, [DefaultKeyFunc] inspects message metadata
// ([msg.MetaKeyDeduplicateId] followed by [msg.MetaKeyMessageId]).
// For protobuf payloads whose business identifier resides in the payload itself, [ProtoKeyFunc] unmarshals
// the message and reads the specified field.
// A key longer than [DefaultMaxKeyLength] bytes ([WithMaxKeyLength]) terminates its message with [ErrKeyTooLong]
// before it reaches the storage.
//
// # Trust model
//
// The key is chosen by the producer, and the broker carries no authenticated producer identity to bind it to: any
// publisher allowed on a subject can send a message with the key of a later message on that subject, which is then
// acknowledged as a duplicate. Records are scoped by consumer and by the subject the message arrived on
// ([BuildStorageKey]), so a publisher on one subject never suppresses a message of another, even under a wildcard
// subscription. Within one subject every permitted publisher is trusted: restrict publish permissions per subject,
// or derive the key from a value the producer cannot forge.
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
