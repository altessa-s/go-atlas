// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package providers

import (
	"context"
	"time"
)

// Prober reports whether a provider is fit to serve. Implementations
// may probe transport reachability or any other invariant that gates
// readiness. Prober is intentionally separate from [Provider] so
// third-party implementations stay backwards-compatible: Uniq probes
// via type assertion and assumes Serving when the assertion fails.
//
// All in-tree providers (nats, redis, noop) implement Prober.
type Prober interface {
	// Probe returns nil when the provider is healthy, or an error
	// describing the failure otherwise.
	Probe(ctx context.Context) error
}

// Provider defines the interface that must be implemented by any storage backend
// used with the uniq package. The interface provides methods for managing unique
// values with support for TTL (Time To Live).
type Provider interface {
	// Add adds a key to the storage.
	// If the key already exists, it will be overwritten with the new TTL.
	Add(ctx context.Context, key string) error

	// AddWithValue adds a key with an associated value.
	AddWithValue(ctx context.Context, key string, value []byte) error

	// TryAdd atomically adds a key only if it does not already exist.
	// Returns (true, nil) when the key was inserted, (false, nil) when it
	// was already present, and (false, err) on storage failure. Use this
	// instead of Add when you need race-free "first writer wins" semantics
	// (e.g. one-time-token redemption, idempotent webhook handling).
	//
	// ttl overrides the provider's configured TTL for this key. Pass
	// zero (or negative) to use the provider/bucket default.
	TryAdd(ctx context.Context, key string, ttl time.Duration) (bool, error)

	// TryAddWithValue is like TryAdd but stores an associated value when
	// the insert succeeds. The value is ignored when the key is already
	// present.
	//
	// ttl overrides the provider's configured TTL for this key. Pass
	// zero (or negative) to use the provider/bucket default.
	TryAddWithValue(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)

	// Exist checks if a key exists in the storage.
	// Returns true if the key exists and hasn't expired, false otherwise.
	Exist(ctx context.Context, key string) (bool, error)

	// GetValue retrieves the value associated with a key in the storage.
	GetValue(ctx context.Context, key string) ([]byte, error)

	// Remove removes a key from the storage.
	// If the key doesn't exist, no error is returned.
	Remove(ctx context.Context, key string) error

	// Clear removes all keys from the storage.
	// The operation should be atomic when possible.
	Clear(ctx context.Context) error
}
