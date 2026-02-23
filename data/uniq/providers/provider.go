// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package providers

import (
	"context"
)

// Provider defines the interface that must be implemented by any storage backend
// used with the uniq package. The interface provides methods for managing unique
// values with support for TTL (Time To Live).
type Provider interface {
	// Add adds a key to the storage.
	// If the key already exists, it will be overwritten with the new TTL.
	Add(ctx context.Context, key string) error

	// AddWithValue adds a key with an associated value.
	AddWithValue(ctx context.Context, key string, value []byte) error

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
