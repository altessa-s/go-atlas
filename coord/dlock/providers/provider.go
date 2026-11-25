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

// Prober reports whether a provider is fit to serve. Implementations
// may probe anything that gates readiness: transport reachability
// (e.g. NATS connection state), required resources (e.g. KV bucket
// accessibility), or any other invariant that must hold before the
// provider can fulfill [Provider.Lock] requests.
//
// Prober is intentionally separate from [Provider] so third-party
// implementations can opt into probing: DLock probes via type assertion
// and assumes Serving when the assertion fails.
//
// All in-tree providers (nats, noop) implement Prober.
type Prober interface {
	// Probe returns nil when the provider is healthy, or an error
	// describing the failure otherwise.
	Probe(ctx context.Context) error
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

// LostNotifier is an optional interface of a [Lock] that learns when its
// lease ends without a Release. DLock.Synchronize uses it to cancel the
// function it runs; a lock without it keeps running to the end.
type LostNotifier interface {
	// Lost returns a channel that is closed once the lock no longer holds
	// its lease although it was not released: the lease expired, was taken
	// over or could not be renewed in time.
	Lost() <-chan struct{}
}

// LockInfo is the information about the current state of the lock.
type LockInfo struct {
	Key          string        // The key of the lock.
	Owner        string        // The owner of the lock.
	AcquiredAt   time.Time     // The time the lock was acquired.
	LastRenewed  time.Time     // The time the lock was last renewed.
	TTL          time.Duration // The time-to-live duration of the lock.
	FencingToken uint64        // Monotonically increasing token for fencing stale lock holders.

	// IsStale is a BEST-EFFORT reporting hint computed from
	// time.Now().Sub(LastRenewed) > TTL. The subtraction uses non-monotonic
	// wall-clock fields that may differ between the holder and the
	// observer node, so IsStale can flip across NTP corrections or
	// clock skew — callers MUST NOT use it to make safety-critical
	// decisions. Use the provider's lease
	// mechanism to establish ownership and FencingToken to order fenced writes.
	IsStale bool
}
