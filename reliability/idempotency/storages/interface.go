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
	// Pass lockTtl <= 0 to fall back to the configured TTL.
	//
	// Use this when different keys need different lock lifetimes
	// (e.g. short-lived OTP tokens vs long-running webhook
	// processing).
	AttemptLockWithTTL(ctx context.Context, key string, val []byte, lockTtl time.Duration) (acquired bool, existing []byte, lockToken []byte, err error)

	// Complete writes val for key when lockToken still matches the
	// current state (CAS). Returns [ErrLockStolen] when the lock has
	// been taken over by another holder.
	Complete(ctx context.Context, key string, val []byte, lockToken []byte) error

	// Steal atomically replaces the existing key value when its
	// current content equals expectedVal. Returns the new lockToken
	// for the caller to use as the holder of the freshly-stolen lock.
	//
	// Returns [ErrLockStolen] when expectedVal no longer matches the
	// current value (somebody else stole the lock between the caller's
	// observation and this Steal call).
	//
	// Used by orphan reclaim: when an InProgress entry's timestamp
	// indicates the original holder has crashed (older than the
	// configured maxLockDuration), Keeper builds a fresh InProgress
	// payload and calls Steal to take ownership.
	Steal(ctx context.Context, key string, expectedVal, newVal []byte) ([]byte, error)

	// Release atomically deletes key only if lockToken still owns it.
	// Missing or replaced keys return ErrLockStolen; nil tokens return ErrMissingLockState.
	Release(ctx context.Context, key string, lockToken []byte) error

	// Delete unconditionally removes a key for administrative use.
	// Request error cleanup must use Release instead.
	Delete(ctx context.Context, key string) error
}
