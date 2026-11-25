// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import "slices"

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
// by [Keeper.AttemptLock] when a caller acquires the lock and consumed
// by [Keeper.Complete] to detect stolen locks via CAS.
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

	// fingerprint and owner are the request fingerprint and the owner
	// recorded with the lock (see the Keeper's AttemptLockOpts.Fingerprint
	// and WithOwner); the Keeper keeps them on the record when completing.
	fingerprint []byte `json:"-"`
	owner       string `json:"-"`
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

// Fingerprint returns a copy of the request fingerprint recorded with the
// key, nil when none was.
func (s *State) Fingerprint() []byte {
	if s == nil {
		return nil
	}
	return slices.Clone(s.fingerprint)
}

// SetFingerprint records the request fingerprint on this state. Used by the
// Keeper; ordinary callers don't invoke it.
func (s *State) SetFingerprint(fp []byte) {
	if s == nil {
		return
	}
	s.fingerprint = slices.Clone(fp)
}

// Owner returns the owner recorded with the key, "" when none was.
func (s *State) Owner() string {
	if s == nil {
		return ""
	}
	return s.owner
}

// SetOwner records the owner on this state. Used by the Keeper; ordinary
// callers don't invoke it.
func (s *State) SetOwner(owner string) {
	if s == nil {
		return
	}
	s.owner = owner
}
