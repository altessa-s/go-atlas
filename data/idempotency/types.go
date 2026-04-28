// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"time"
)

// StorageFunc is an adapter to allow the use of ordinary functions as
// a [storages.Storage]. The WithTTL fields are optional: when nil, the
// non-TTL fields are invoked with ttl=0 ignored.
type StorageFunc struct {
	AttemptLockFunc        func(ctx context.Context, key string, val []byte) (bool, []byte, []byte, error)
	AttemptLockWithTTLFunc func(ctx context.Context, key string, val []byte, lockTtl time.Duration) (bool, []byte, []byte, error)
	CompleteFunc           func(ctx context.Context, key string, val []byte, lockToken []byte) error
	CompleteWithTTLFunc    func(ctx context.Context, key string, val []byte, lockToken []byte, resultTtl time.Duration) error
	DeleteFunc             func(ctx context.Context, key string) error
	// SupportsAttemptLockWithTTLFunc lets tests model backend-specific
	// capability. When nil, [StorageFunc.SupportsAttemptLockWithTTL]
	// reports true.
	SupportsAttemptLockWithTTLFunc func() bool
	// SupportsCompleteWithTTLFunc lets tests model backend-specific
	// capability. When nil, [StorageFunc.SupportsCompleteWithTTL]
	// reports true.
	SupportsCompleteWithTTLFunc func() bool
}

// AttemptLock implements the Storage interface.
func (f StorageFunc) AttemptLock(ctx context.Context, key string, val []byte) (bool, []byte, []byte, error) {
	return f.AttemptLockFunc(ctx, key, val)
}

// AttemptLockWithTTL implements the Storage interface. Falls back to
// AttemptLockFunc (ignoring lockTtl) when no WithTTL function is set.
func (f StorageFunc) AttemptLockWithTTL(ctx context.Context, key string, val []byte, lockTtl time.Duration) (bool, []byte, []byte, error) {
	if f.AttemptLockWithTTLFunc != nil {
		return f.AttemptLockWithTTLFunc(ctx, key, val, lockTtl)
	}
	return f.AttemptLockFunc(ctx, key, val)
}

// Complete implements the Storage interface.
func (f StorageFunc) Complete(ctx context.Context, key string, val []byte, lockToken []byte) error {
	return f.CompleteFunc(ctx, key, val, lockToken)
}

// CompleteWithTTL implements the Storage interface. Falls back to
// CompleteFunc (ignoring resultTtl) when no WithTTL function is set.
func (f StorageFunc) CompleteWithTTL(ctx context.Context, key string, val []byte, lockToken []byte, resultTtl time.Duration) error {
	if f.CompleteWithTTLFunc != nil {
		return f.CompleteWithTTLFunc(ctx, key, val, lockToken, resultTtl)
	}
	return f.CompleteFunc(ctx, key, val, lockToken)
}

// Delete implements the Storage interface.
func (f StorageFunc) Delete(ctx context.Context, key string) error {
	return f.DeleteFunc(ctx, key)
}

// SupportsAttemptLockWithTTL implements the Storage interface.
// Defaults to true when no SupportsAttemptLockWithTTLFunc is set.
func (f StorageFunc) SupportsAttemptLockWithTTL() bool {
	if f.SupportsAttemptLockWithTTLFunc != nil {
		return f.SupportsAttemptLockWithTTLFunc()
	}
	return true
}

// SupportsCompleteWithTTL implements the Storage interface. Defaults
// to true when no SupportsCompleteWithTTLFunc is set.
func (f StorageFunc) SupportsCompleteWithTTL() bool {
	if f.SupportsCompleteWithTTLFunc != nil {
		return f.SupportsCompleteWithTTLFunc()
	}
	return true
}
