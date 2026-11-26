// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides in-memory storage for idempotency keys with TTL support.
// Set a scheduler and a cleanup schedule for automatic cleanup. Without them,
// call [Storage.RunCleanup] to remove expired entries.
//
// The storage clock controls lock age and expiration.
//
// Example:
//
//	storage, err := memory.New(
//		memory.WithTTL(2*time.Hour),
//		memory.WithCleanupSchedule("@every 1m"),
//	)
//	if err != nil {
//		return err
//	}
package memory
