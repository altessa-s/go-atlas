// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package time_test

import (
	"testing"
	"time"

	coretime "github.com/altessa-s/go-atlas/core/time"
)

func BenchmarkTimerStopAndDrainActive(b *testing.B) {
	for b.Loop() {
		t := time.NewTimer(time.Hour)
		coretime.TimerStopAndDrain(t)
	}
}

func BenchmarkTimerStopAndDrainExpired(b *testing.B) {
	for b.Loop() {
		t := time.NewTimer(time.Nanosecond)
		// Give the runtime a chance to fire it without introducing a
		// data-dependent sleep inside the hot loop.
		<-t.C
		coretime.TimerStopAndDrain(t)
	}
}

func BenchmarkTimerStopAndDrainNil(b *testing.B) {
	var sink bool
	for b.Loop() {
		sink = coretime.TimerStopAndDrain(nil)
	}
	_ = sink
}
