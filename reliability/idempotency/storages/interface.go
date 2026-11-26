// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storages

import (
	"context"
	"time"
)

// Storage interface for idempotency key storage.
type Storage interface {
	// AttemptLock tries to acquire a lock for the given key using the
	// backend's configured TTL. Equivalent to
	// AttemptLockWithTTL(ctx, key, val, 0).
	AttemptLock(ctx context.Context, key string, val []byte) (acquired bool, existing []byte, lockToken []byte, err error)

	// AttemptLockWithTTL is like [Storage.AttemptLock] but applies a
	// per-call lockTtl that overrides the backend's configured value.
	// Pass lockTtl <= 0 to fall back to the configured TTL. If a
	// conflicting record disappears before it can be returned, both
	// acquired is false and existing can be nil. The caller may retry.
	//
	// Use this when different keys need different lock lifetimes
	// (e.g. short-lived OTP tokens vs long-running webhook
	// processing).
	AttemptLockWithTTL(ctx context.Context, key string, val []byte, lockTtl time.Duration) (acquired bool, existing []byte, lockToken []byte, err error)

	// Complete writes val for key when lockToken still matches the
	// current state (CAS). Returns [ErrLockStolen] when the lock has
	// expired or been taken over by another holder.
	Complete(ctx context.Context, key string, val []byte, lockToken []byte) error

	// Steal replaces an unexpired lock when its bytes match expected and
	// its backend-stamped age exceeds maxLockAge. A nonpositive maxLockAge
	// imposes no age threshold. A nonpositive ttl uses the backend default.
	// Use a distinct replacement value to fence the prior token on
	// stores that use the payload as the token. On success Steal returns
	// the new token. Otherwise current is the live
	// value observed during the attempt, or nil if the key is absent.
	// Stores that expire locks as leases implement [LockLeaseStorage]
	// and decline Steal while returning the current value, if any.
	Steal(ctx context.Context, key string, expected, replacement []byte, maxLockAge, ttl time.Duration) (stolen bool, current, token []byte, err error)

	// Release atomically deletes key only if lockToken still owns it.
	// Missing or replaced keys return ErrLockStolen; nil tokens return ErrMissingLockState.
	Release(ctx context.Context, key string, lockToken []byte) error

	// Delete unconditionally removes a key for administrative use.
	// Request error cleanup must use Release instead.
	Delete(ctx context.Context, key string) error
}

// LockLeaseStorage is implemented by stores that recover orphaned locks by
// expiring them at the backend. For these stores the Keeper caps the lock TTL
// using the acquiring call's MaxLockDuration and does not call Steal.
// Wrappers must forward this interface for the Keeper to apply that cap.
type LockLeaseStorage interface {
	UsesLockLease() bool
}
