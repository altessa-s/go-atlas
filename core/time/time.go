// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package time

import "time"

// TimerStopAndDrain safely stops a [time.Timer] and drains its channel if the
// timer has already expired, preventing goroutine leaks. It returns true if the
// timer was stopped before firing, and false if the timer had already expired or
// t is nil. A non-blocking select is used for the channel drain so the call
// never blocks, even if another goroutine has already consumed the timer event.
//
// TimerStopAndDrain is safe for concurrent use.
//
// Example:
//
//	timer := stdtime.NewTimer(5 * stdtime.Second)
//	defer TimerStopAndDrain(timer)
func TimerStopAndDrain(t *time.Timer) bool {
	if t == nil {
		return false
	}

	s := t.Stop()
	if !s {
		// Try to drain the channel, but use select to avoid blocking
		// in case someone else has already drained it
		select {
		case <-t.C:
		default:
		}
	}
	return s
}
