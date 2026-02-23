// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package providers

import (
	"context"
	"time"
)

// Provider is the interface that must be implemented by all providers.
type Provider interface {
	// Lock acquires a distributed lock with the provided key.
	// Returns an error if the lock cannot be acquired.
	Lock(ctx context.Context, key string) (Lock, error)

	// GetLockInfo returns information about the current state of the lock.
	GetLockInfo(ctx context.Context, key string) (*LockInfo, error)

	// Close closes the provider and releases all resources.
	// It should be called when the provider is no longer needed.
	// The context can be used to set a timeout for the close operation.
	Close(ctx context.Context) error
}

// Lock is the interface that must be implemented by all locks.
type Lock interface {
	// GetLockInfo returns information about the current state of the lock.
	GetLockInfo(ctx context.Context) (*LockInfo, error)

	// Release releases the lock with context support.
	// The context can be used to set a timeout for the release operation.
	// If the context is canceled, the release operation should be aborted.
	Release(ctx context.Context) error
}

// LockInfo is the information about the current state of the lock.
type LockInfo struct {
	Key          string        // The key of the lock.
	Owner        string        // The owner of the lock.
	AcquiredAt   time.Time     // The time the lock was acquired.
	LastRenewed  time.Time     // The time the lock was last renewed.
	TTL          time.Duration // The time-to-live duration of the lock.
	IsStale      bool          // Whether the lock is stale.
	FencingToken uint64        // Monotonically increasing token for fencing stale lock holders.
}
