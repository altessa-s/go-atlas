// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package dlock provides distributed locks for coordinating access across multiple service instances.
// Supports pluggable providers (NATS JetStream, Redis, No-op) with automatic resource management
// and context-aware operations.
//
// # Features
//
//   - Distributed coordination: ensures only one instance processes a resource at a time.
//   - Pluggable backends: NATS JetStream (recommended), Redis, or No-op (for testing).
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
package dlock
