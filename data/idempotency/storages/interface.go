// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storages

import "context"

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
	// AttemptLock tries to acquire a lock for the given key.
	// It operates atomically:
	//   - If key does not exist: sets key to val and returns
	//     (true, nil, lockToken, nil). lockToken identifies the
	//     caller's claim and must be passed to [Storage.Complete]
	//     to detect stolen locks via CAS.
	//   - If key exists: returns (false, existingVal, nil, nil).
	AttemptLock(ctx context.Context, key string, val []byte) (acquired bool, existing []byte, lockToken []byte, err error)

	// Complete writes val for key when lockToken still matches the
	// current state (CAS). Returns [ErrLockStolen] when the lock has
	// been taken over by another holder (typically because the lock
	// TTL expired and another caller's AttemptLock succeeded in
	// between).
	//
	// lockToken must come from the AttemptLock call that returned
	// acquired=true for this same key.
	Complete(ctx context.Context, key string, val []byte, lockToken []byte) error

	// Delete removes the key from storage (e.g. on failure).
	Delete(ctx context.Context, key string) error
}
