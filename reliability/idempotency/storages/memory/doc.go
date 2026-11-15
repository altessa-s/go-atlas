// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides in-memory storage for idempotency keys with TTL support.
// Suitable for single-instance deployments with automatic cleanup. Not persistent across restarts.
//
// Example:
//
//	storage := memory.New(
//		memory.WithTTL(2*time.Hour),
//		memory.WithCleanupSchedule("@every 1m"),
//	)
//	interceptor := idempotency.ServerInterceptor(storage)
package memory
