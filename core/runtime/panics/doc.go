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
// # Guarded channel sends
//
// TrySend and TrySendNonBlocking send on a channel while recovering the
// "send on closed channel" panic, reporting (sent, closed) instead of
// crashing. Needing them is a channel-ownership smell — the sender should
// own the close — but they contain the crash when that ownership is shared
// or inverted.
//
//	if _, closed := panics.TrySendNonBlocking(ch, ev); closed {
//	    log.Warn("subscriber channel closed during send")
//	}
//
// # Configuration
//
//   - SetReallyPanic(bool): Configure whether panics re-propagate after handling (default: false).
//   - SetLogger(*slog.Logger): Set custom logger for default panic handler.
//   - AddGlobalPanicHandler(PanicHandler): Add to global handlers.
package panics
