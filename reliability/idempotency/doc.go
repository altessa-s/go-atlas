// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package idempotency provides duplicate request detection using idempotency keys.
// It supports memory, Redis, NATS, MongoDB and SQL storage backends. MongoDB
// and SQL can commit keys with business writes in the caller's transaction.
// SQL storage purges expired keys lazily in the background; PurgeExpired(ctx)
// also drains expired rows manually and returns an int64 count.
//
// Redis, MongoDB, SQL, and memory use their own clocks for orphan takeover.
// NATS KV delegates the age check to the Keeper and requires synchronized
// Keeper clocks.
//
// // # Ownership
//
// [Keeper.AttemptLock] returns a [State] with an opaque ownership token. Pass that State to
// Complete on success or Release on failure. Both reject a stale owner's token.
// Delete is unconditional administrative invalidation, not request cleanup.
//
// [WithOwner] binds each record to its caller. The optional
// [reliability/idempotency/principalowner.PrincipalOwner](../../reliability/idempotency/principalowner#principalowner) adapter labels it with the verified
// principal of the request. The transports replay a response or echo an entity
// ID only for a record bound to an owner.
//
// # Storage keys
//
// [ComposeKey] encodes an ordered tuple into one storage key in the
// length-prefixed v2 format, so distinct tuples never share a key. The HTTP
// and gRPC transports compose ("transport", service, key), the broker
// middleware ("ibk", consumer, topic, key), and the Keeper wraps every key it is
// given as ("namespace", namespace, key) — even in the empty namespace a nil
// [KeyNamespaceFunc] selects — before any storage operation.
//
// # Usage
//
//	storage, err := memory.New()
//	if err != nil {
//		return err
//	}
//	keeper := idempotency.New(storage)
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
