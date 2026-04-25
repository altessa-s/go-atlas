// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import "time"

// Common validation constants used across all providers
const (
	// MaxKeyLength defines the maximum allowed length for secret keys across all providers.
	// This limit ensures compatibility with storage backends and prevents excessive memory usage.
	// Keys longer than this limit will be rejected with ErrInvalidKey.
	MaxKeyLength = 255

	// MinKeyLength defines the minimum allowed length for secret keys across all providers.
	// This prevents empty or overly short keys that could cause collisions or storage issues.
	// Keys shorter than this limit will be rejected with ErrInvalidKey.
	MinKeyLength = 2
)

// Graceful shutdown constants for coordinating Manager lifecycle
const (
	// DefaultShutdownTimeout is the default timeout duration for graceful shutdown operations.
	// This provides a reasonable balance between allowing sufficient time for cleanup while
	// preventing indefinite hangs during application shutdown. Used by Shutdown() method.
	DefaultShutdownTimeout = 30 * time.Second

	// MinShutdownTimeout is the minimum allowed timeout for shutdown operations.
	// Timeouts below this value are rejected to ensure sufficient time for basic cleanup
	// operations like stopping background goroutines and clearing sensitive data.
	MinShutdownTimeout = 1 * time.Second

	// MaxShutdownTimeout is the maximum allowed timeout for shutdown operations.
	// This prevents unreasonably long shutdown timeouts that could delay application
	// termination. Extended timeouts may indicate underlying system issues.
	MaxShutdownTimeout = 10 * time.Minute
)
