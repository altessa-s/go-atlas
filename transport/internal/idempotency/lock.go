// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"errors"

	idempotencydata "github.com/altessa-s/go-atlas/data/idempotency"
)

// LockOutcome classifies the result of an AttemptLock call for a transport,
// which answers each outcome its own way (status, headers, replay).
type LockOutcome int

const (
	// LockAcquired means the request holds the lock: the transport runs it,
	// then completes or releases the key with the returned state.
	LockAcquired LockOutcome = iota
	// LockFingerprintMismatch means the key was recorded for a different
	// request ([idempotencydata.ErrFingerprintMismatch]).
	LockFingerprintMismatch
	// LockOwnerMismatch means the caller may not see the key's record
	// ([idempotencydata.ErrOwnerMismatch]); the error may carry a
	// transport status from a ReplayAuthorizer.
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
func ClassifyLock(locked bool, state *idempotencydata.State, err error) (LockOutcome, Record) {
	switch {
	case errors.Is(err, idempotencydata.ErrFingerprintMismatch):
		return LockFingerprintMismatch, Record{}
	case errors.Is(err, idempotencydata.ErrOwnerMismatch):
		return LockOwnerMismatch, Record{}
	case err != nil:
		return LockStorageError, Record{}
	case locked:
		return LockAcquired, Record{}
	case state == nil:
		return LockNilState, Record{}
	case state.Status == idempotencydata.StatusInProgress:
		return LockInProgress, Record{}
	case state.Status == idempotencydata.StatusSuccess:
		record, _ := DecodeRecord(state.Data)
		return LockCompleted, record
	default:
		return LockAcquired, Record{}
	}
}
