// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"time"
)

// StorageFunc is an adapter to allow the use of ordinary functions as
// a [storages.Storage]. The AttemptLockWithTTLFunc field is optional:
// when nil, AttemptLockFunc is invoked and lockTtl is ignored.
// StealFunc is required when the caller exercises the orphan-reclaim
// path; otherwise it can stay nil.
type StorageFunc struct {
	AttemptLockFunc        func(ctx context.Context, key string, val []byte) (bool, []byte, []byte, error)
	AttemptLockWithTTLFunc func(ctx context.Context, key string, val []byte, lockTtl time.Duration) (bool, []byte, []byte, error)
	CompleteFunc           func(ctx context.Context, key string, val []byte, lockToken []byte) error
	StealFunc              func(ctx context.Context, key string, expectedVal, newVal []byte) ([]byte, error)
	DeleteFunc             func(ctx context.Context, key string) error
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

// Steal implements the Storage interface.
func (f StorageFunc) Steal(ctx context.Context, key string, expectedVal, newVal []byte) ([]byte, error) {
	return f.StealFunc(ctx, key, expectedVal, newVal)
}

// Delete implements the Storage interface.
func (f StorageFunc) Delete(ctx context.Context, key string) error {
	return f.DeleteFunc(ctx, key)
}
