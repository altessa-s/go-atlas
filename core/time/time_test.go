// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package time_test

import (
	"testing"
	"time"

	coretime "github.com/altessa-s/go-atlas/core/time"
)

func TestTimerStopAndDrain(t *testing.T) {
	t.Run("Stop active timer", func(t *testing.T) {
		timer := time.NewTimer(time.Hour)
		if !coretime.TimerStopAndDrain(timer) {
			t.Error("TimerStopAndDrain should return true for active timer")
		}
	})

	t.Run("Stop expired timer", func(t *testing.T) {
		timer := time.NewTimer(time.Nanosecond)
		time.Sleep(10 * time.Millisecond) // Let it expire

		// After expiry, TimerStopAndDrain should not panic and should drain the channel.
		// Stop() returns false for an expired timer, so TimerStopAndDrain returns false.
		// However, due to timing, Stop() might still return true if called before
		// the runtime delivers the timer event, so we only verify no panic and
		// that the channel is drained afterward.
		coretime.TimerStopAndDrain(timer)

		// Verify channel is empty (drained by TimerStopAndDrain or already read)
		select {
		case <-timer.C:
			t.Error("Channel should be drained after TimerStopAndDrain")
		default:
		}
	})

	t.Run("Nil timer", func(t *testing.T) {
		if coretime.TimerStopAndDrain(nil) {
			t.Error("TimerStopAndDrain(nil) should return false")
		}
	})
}
