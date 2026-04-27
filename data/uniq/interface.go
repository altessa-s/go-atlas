// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uniq

import (
	"context"
	"time"
)

// Uniquer is the interface that wraps the basic methods for managing unique values.
type Uniquer interface {
	// Add adds a key to the uniq set.
	Add(ctx context.Context, key string) error

	// AddWithValue adds a key with an associated value to the uniq set.
	AddWithValue(ctx context.Context, key string, value any) error

	// TryAdd atomically inserts a key only if it does not already exist.
	// Returns (true, nil) when the key was inserted (race won),
	// (false, nil) when the key was already present, and (false, err)
	// on storage failure. Use this instead of [Uniquer.Add] when you
	// need race-free first-writer-wins semantics.
	//
	// ttl overrides the per-key TTL for this call. Pass zero to use
	// the value configured on the underlying provider/bucket.
	TryAdd(ctx context.Context, key string, ttl time.Duration) (bool, error)

	// TryAddWithValue is like [Uniquer.TryAdd] but stores an associated
	// value when the insert succeeds. The value is ignored when the key
	// already exists.
	//
	// ttl overrides the per-key TTL for this call. Pass zero to use
	// the value configured on the underlying provider/bucket.
	TryAddWithValue(ctx context.Context, key string, value any, ttl time.Duration) (bool, error)

	// GetValue retrieves the value associated with a key in the uniq set.
	GetValue(ctx context.Context, key string, out any) error

	// Exist checks if a key exists in the uniq set.
	Exist(ctx context.Context, key string) (bool, error)

	// Remove removes a key from the uniq set.
	Remove(ctx context.Context, key string) error

	// Clear clears the uniq set.
	Clear(ctx context.Context) error
}
