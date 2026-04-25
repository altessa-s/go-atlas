// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
)

// StorageFunc is an adapter to allow the use of ordinary functions as Idempotency.
type StorageFunc struct {
	AttemptLockFunc func(ctx context.Context, key string, val []byte) (bool, []byte, error)
	CompleteFunc    func(ctx context.Context, key string, val []byte) error
	DeleteFunc      func(ctx context.Context, key string) error
}

// AttemptLock implements the Idempotency interface.
func (f StorageFunc) AttemptLock(ctx context.Context, key string, val []byte) (bool, []byte, error) {
	return f.AttemptLockFunc(ctx, key, val)
}

// Complete implements the Idempotency interface.
func (f StorageFunc) Complete(ctx context.Context, key string, val []byte) error {
	return f.CompleteFunc(ctx, key, val)
}

// Delete implements the Idempotency interface.
func (f StorageFunc) Delete(ctx context.Context, key string) error {
	return f.DeleteFunc(ctx, key)
}
