// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package idempotency provides duplicate request detection using idempotency keys.
// It supports memory, Redis, and NATS storage backends.
//
// # Ownership
//
// AttemptLock returns a State with an opaque ownership token. Pass that State to
// Complete on success or Release on failure. Both reject a stale owner's token.
// Delete is unconditional administrative invalidation, not request cleanup.
//
// # Storage keys
//
// [ComposeKey] encodes an ordered tuple into one storage key in the
// length-prefixed v2 format, so distinct tuples never share a key. The HTTP
// and gRPC transports compose ("transport", service, key), the broker
// middleware ("ibk", consumer, key), and the Keeper wraps every key it is
// given as ("namespace", namespace, key) — even in the empty namespace a nil
// [KeyNamespaceFunc] selects — before any storage operation.
//
// # Rolling out the v2 key format
//
// The v2 format replaced ':'-joined keys that let distinct tuples share a
// record. The switch is a clean cut: v1 records are never read, copied or
// deleted. Point the v2 release at a new Redis key prefix, NATS KV bucket
// or equivalent storage namespace. While old and new replicas run side by
// side, and until the old records expire, their locks are independent: the
// same operation can run on both versions, and one completed by an old
// replica can run again. A rollback is another such transition. Where
// duplicates are unacceptable, quiesce intake, drain handlers and wait out
// the old retention window before resuming, or rely on durable
// business-level uniqueness.
//
// # Usage
//
//	keeper := idempotency.New(memory.New())
//	locked, state, err := keeper.AttemptLock(ctx, "request-123")
//	if err != nil || !locked {
//	    return err
//	}
//	result, err := doWork(ctx)
//	if err != nil {
//	    _ = keeper.Release(ctx, "request-123", state)
//	    return err
//	}
//	return keeper.Complete(ctx, "request-123", result, state)
package idempotency
