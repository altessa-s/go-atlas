// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package dlock provides distributed locks for coordinating access across multiple service instances.
// Supports pluggable providers (NATS JetStream, MongoDB, SQL, No-op) with automatic resource management
// and context-aware operations.
//
// # Features
//
//   - Distributed coordination: ensures only one instance processes a resource at a time.
//   - Pluggable backends: NATS JetStream (recommended), MongoDB, SQL, or No-op (for testing).
//   - Resource management: automatic lock release and cleanup.
//   - Context awareness: supports cancellation and timeouts for lock acquisition.
//
// # Usage
//
//	dl := dlock.New(provider)
//	defer dl.Close()
//
//	err := dl.Synchronize(ctx, "resource-key", func(ctx context.Context) error {
//	    // This block is executed only if the lock is acquired.
//	    // The lock is automatically released when the function returns.
//	    return doWork(ctx)
//	})
//
// # Lost locks
//
// Synchronize cancels the context it passes to the function, with cause
// [ErrLockLost], when the lease is lost while the function runs, and then
// returns an error matching [ErrLockLost]. Only locks implementing
// providers.LostNotifier (the mongo and sqldb providers) report a loss; with
// the others the function runs to the end. The release that follows is
// detached from the caller's cancellation and bounded by [WithReleaseTimeout].
// Synchronize wraps [ErrPanicked] with the panic's Go kind and joins any release
// error with the other errors.
package dlock
