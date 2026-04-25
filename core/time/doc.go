// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package time provides utilities for safe timer management.
// Extends the standard library with helpers to prevent goroutine leaks
// by safely stopping timers and draining channels.
//
// All exported functions are thread-safe. TimerStopAndDrain uses
// non-blocking channel drain to avoid race conditions.
//
// # Usage
//
//	timer := stdtime.NewTimer(5 * stdtime.Second)
//
//	// Later, safely stop the timer
//	wasActive := time.TimerStopAndDrain(timer)
//	if wasActive {
//	    // Timer was stopped before expiring
//	} else {
//	    // Timer had already expired, channel drained
//	}
//
// # Performance
//
// No notable overhead beyond standard library operations. Uses select
// with default case for non-blocking channel drain.
package time
