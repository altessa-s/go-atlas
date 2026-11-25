// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import "errors"

// LockStatus is the status reported by a transport's lock attempt.
type LockStatus string

const (
	StatusInProgress LockStatus = "IN_PROGRESS"
	StatusSuccess    LockStatus = "SUCCESS"
)

// LockErrorKind distinguishes errors that require different transport responses.
type LockErrorKind uint8

const (
	NoLockError LockErrorKind = iota
	FingerprintMismatch
	OwnerMismatch
	StorageError
)

// ClassifyLockError recognizes the caller's sentinels, including wrapped errors.
func ClassifyLockError(err, fingerprintMismatch, ownerMismatch error) LockErrorKind {
	switch {
	case err == nil:
		return NoLockError
	case errors.Is(err, fingerprintMismatch):
		return FingerprintMismatch
	case errors.Is(err, ownerMismatch):
		return OwnerMismatch
	default:
		return StorageError
	}
}

// LockOutcome classifies the result of an AttemptLock call for a transport,
// which answers each outcome its own way (status, headers, replay).
type LockOutcome int

const (
	// LockAcquired means the request holds the lock: the transport runs it,
	// then completes or releases the key with the returned state.
	LockAcquired LockOutcome = iota
	// LockFingerprintMismatch means the key was recorded for a different request.
	LockFingerprintMismatch
	// LockOwnerMismatch means the caller may not see the key's record.
	// The error may carry a transport status from a ReplayAuthorizer.
	LockOwnerMismatch
	// LockStorageError means the storage failed; the transport applies its
	// fallback behavior.
	LockStorageError
	// LockNilState means the lock was not acquired but no state came back.
	LockNilState
	// LockInProgress means another request holds the key.
	LockInProgress
	// LockCompleted means the key was completed; the returned [Record]
	// holds the data stored with it.
	LockCompleted
)

// ClassifyLock classifies the locked, state and err an AttemptLock call
// returned. For [LockCompleted] it also returns the record decoded from the
// state's data by [DecodeRecord], the zero Record otherwise. A state that is
// neither in progress nor completed is [LockAcquired], as the transports
// have always treated it.
func ClassifyLock(locked, hasState bool, status LockStatus, data any, errKind LockErrorKind) (LockOutcome, Record) {
	switch {
	case errKind == FingerprintMismatch:
		return LockFingerprintMismatch, Record{}
	case errKind == OwnerMismatch:
		return LockOwnerMismatch, Record{}
	case errKind != NoLockError:
		return LockStorageError, Record{}
	case locked:
		return LockAcquired, Record{}
	case !hasState:
		return LockNilState, Record{}
	case status == StatusInProgress:
		return LockInProgress, Record{}
	case status == StatusSuccess:
		record, _ := DecodeRecord(data)
		return LockCompleted, record
	default:
		return LockAcquired, Record{}
	}
}
