// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package base

import (
	"context"

	"github.com/altessa-s/go-atlas/security/secrets"
)

// WithLock executes the given function with optional distributed locking.
// If locker is nil, the function is executed directly without locking.
// If locker is provided, the function is executed within a synchronized block.
//
// Parameters:
//   - ctx: context for the operation
//   - locker: optional distributed locker (can be nil)
//   - providerName: name of the provider (used for lock key prefix)
//   - encodedKey: the encoded key to lock on
//   - fn: the function to execute
//
// Returns the error from the function or from the locker.
func WithLock(
	ctx context.Context,
	locker secrets.Locker,
	providerName string,
	encodedKey string,
	fn func(ctx context.Context) error,
) error {
	if locker == nil {
		return fn(ctx)
	}

	lockKey := secrets.CreateLockKey(providerName, encodedKey)
	return locker.Synchronize(ctx, lockKey, fn)
}

// WithLockResult executes the given function with optional distributed locking
// and returns both the result and error.
// If locker is nil, the function is executed directly without locking.
//
// Type parameter T is the return type of the function.
//
// Parameters:
//   - ctx: context for the operation
//   - locker: optional distributed locker (can be nil)
//   - providerName: name of the provider (used for lock key prefix)
//   - encodedKey: the encoded key to lock on
//   - fn: the function to execute
//
// Returns the result and error from the function.
func WithLockResult[T any](
	ctx context.Context,
	locker secrets.Locker,
	providerName string,
	encodedKey string,
	fn func(ctx context.Context) (T, error),
) (T, error) {
	var result T
	var resultErr error

	err := WithLock(ctx, locker, providerName, encodedKey, func(ctx context.Context) error {
		result, resultErr = fn(ctx)
		return resultErr
	})

	if err != nil {
		return result, err
	}

	return result, resultErr
}
