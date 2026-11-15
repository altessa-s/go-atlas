// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package idempotencytest provides MockIdempotencyStorage, a concurrency-safe in-memory idempotency storage
// for tests of idempotency consumers (HTTP and gRPC middleware, broker deduplication).
//
// Example:
//
//	st := idempotencytest.NewMockIdempotencyStorage()
package idempotencytest
