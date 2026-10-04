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
