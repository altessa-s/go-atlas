// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlock

import (
	"context"

	"github.com/altessa-s/go-atlas/data/locks/dlock/providers"
)

// Locker defines the interface for distributed locking mechanisms.
// Implementations should provide thread-safe methods for acquiring and managing locks.
type Locker interface {
	// Lock makes a single attempt to acquire a distributed lock for key.
	// It does not wait for a current holder: when the key is taken the
	// attempt fails immediately, so callers that need to be serialized
	// rather than rejected must retry.
	//
	// The context scopes the lock's lifetime — canceling it releases the
	// lock — so it must not end before the work the lock guards.
	Lock(ctx context.Context, key string) (providers.Lock, error)

	// Synchronize acquires a distributed lock for the specified key and
	// executes the provided function while holding the lock, and then releases the lock.
	// This ensures that the function is executed in a mutually exclusive manner across
	// distributed instances. If the lock cannot be acquired, an error is returned.
	// The context is used to manage the lock's lifecycle and can be canceled to release the lock early.
	Synchronize(ctx context.Context, key string, fn func(ctx context.Context) error) error

	// GetLockInfo returns information about the current state of the lock.
	GetLockInfo(ctx context.Context, key string) (*providers.LockInfo, error)
}
