// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storages

import (
	"context"
	"time"
)

// Status represents the state of an idempotency key.
type Status string

const (
	// StatusInProgress indicates the request is currently being processed.
	StatusInProgress Status = "IN_PROGRESS"
	// StatusSuccess indicates the request was completed successfully.
	StatusSuccess Status = "SUCCESS"
)

// State holds the current status and metadata of an idempotency key.
//
// The lockToken field is unexported and not serialized. It is populated
// by [Storage.AttemptLock] when a caller acquires the lock and consumed
// by [Storage.Complete] to detect stolen locks via CAS.
type State struct {
	Status Status `json:"status"`
	Data   any    `json:"data,omitempty"`

	// lockToken is the opaque CAS token returned by AttemptLock.
	// Each backend encodes its own representation:
	//   - memory / redis: a copy of the value bytes written at lock time
	//   - nats: 8-byte big-endian encoded KV revision
	// nil when the State was deserialized for a non-acquired view
	// (i.e. the caller observed an existing lock held by someone else).
	lockToken []byte `json:"-"`
}

// LockToken returns the opaque CAS token associated with this state.
// Used by adapters that need to bridge State values into raw Storage
// calls. Most callers pass *State directly to higher-level APIs and
// don't touch this method.
func (s *State) LockToken() []byte {
	if s == nil {
		return nil
	}
	return s.lockToken
}

// SetLockToken stores the opaque CAS token on this state. Used by
// Storage implementations and adapters; ordinary callers don't invoke it.
func (s *State) SetLockToken(token []byte) {
	if s == nil {
		return
	}
	s.lockToken = token
}

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

	// Delete removes the key from storage (e.g. on failure).
	Delete(ctx context.Context, key string) error
}
