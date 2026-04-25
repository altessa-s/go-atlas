// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package panics provides utilities for panic handling and runtime assertions.
// Includes panic recovery with customizable handlers and assertion functions.
//
// All configuration functions use atomic operations and are thread-safe.
// Handle and HandleWithOpts are safe to use concurrently.
//
// # Basic Usage
//
//	func riskyOperation(ctx context.Context) {
//	    defer panics.Handle(ctx)
//
//	    // Assertions
//	    panics.MustNonNil(config, "config is required")
//	    panics.Must(port > 0, "port must be positive")
//
//	    // Error to panic
//	    user, err := fetchUser(userID)
//	    panics.MustError(err)
//	}
//
// # Configuration
//
//   - SetReallyPanic(bool): Configure whether panics re-propagate after handling (default: false).
//   - SetLogger(*slog.Logger): Set custom logger for default panic handler.
//   - AddGlobalPanicHandler(PanicHandler): Add to global handlers.
package panics
