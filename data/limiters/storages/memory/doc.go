// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides in-memory rate limit storage for single-instance tokenbucket deployments.
// Uses sliding window algorithm with automatic cleanup and thread-safe concurrent access.
//
// Example:
//
//	storage := memory.New(memory.WithCleanupSchedule("@every 5m"))
//	defer storage.Close()
package memory
