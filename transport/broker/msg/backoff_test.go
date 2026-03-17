// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package msg

import (
	"testing"
	"time"
)

func TestScheduleBackOff(t *testing.T) {
	delays := []time.Duration{5 * time.Second, 30 * time.Second, 5 * time.Minute}
	fn := ScheduleBackOff(delays...)

	tests := []struct {
		attempt uint64
		want    time.Duration
	}{
		{0, 5 * time.Second},
		{1, 5 * time.Second},
		{2, 30 * time.Second},
		{3, 5 * time.Minute},
		{4, 5 * time.Minute},  // clamps to last
		{10, 5 * time.Minute}, // clamps to last
	}

	for _, tt := range tests {
		if got := fn(tt.attempt); got != tt.want {
			t.Errorf("ScheduleBackOff(attempt=%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestScheduleBackOff_SingleDelay(t *testing.T) {
	fn := ScheduleBackOff(10 * time.Second)
	for _, attempt := range []uint64{0, 1, 2, 5} {
		if got := fn(attempt); got != 10*time.Second {
			t.Errorf("ScheduleBackOff(attempt=%d) = %v, want %v", attempt, got, 10*time.Second)
		}
	}
}

func TestScheduleBackOff_PanicsWithNoDelays(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic with no delays")
		}
	}()
	ScheduleBackOff()
}

func TestScheduleBackOff_DoesNotMutateInput(t *testing.T) {
	delays := []time.Duration{time.Second, 2 * time.Second}
	fn := ScheduleBackOff(delays...)

	delays[0] = 999 * time.Hour // mutate original slice
	if got := fn(1); got != time.Second {
		t.Fatalf("ScheduleBackOff was affected by caller mutation: got %v", got)
	}
}

func TestExponentialBackOff_Growth(t *testing.T) {
	fn := ExponentialBackOff(time.Second, time.Minute, 0)

	prev := fn(1)
	for attempt := uint64(2); attempt <= 5; attempt++ {
		got := fn(attempt)
		if got < prev {
			t.Errorf("attempt %d: %v < previous %v (expected growth)", attempt, got, prev)
		}
		prev = got
	}
}

func TestExponentialBackOff_ClampsToMaxDelay(t *testing.T) {
	maxDelay := 10 * time.Second
	fn := ExponentialBackOff(time.Second, maxDelay, 0)

	for _, attempt := range []uint64{20, 50, 100} {
		if got := fn(attempt); got > maxDelay {
			t.Errorf("attempt %d: %v exceeds maxDelay %v", attempt, got, maxDelay)
		}
	}
}

func TestExponentialBackOff_JitterWithinBounds(t *testing.T) {
	base := time.Second
	maxDelay := time.Minute
	jitter := 0.5
	fn := ExponentialBackOff(base, maxDelay, jitter)

	for i := range 100 {
		got := fn(uint64(i%5) + 1)
		if got > maxDelay {
			t.Errorf("iteration %d: %v exceeds maxDelay %v", i, got, maxDelay)
		}
		if got < 0 {
			t.Errorf("iteration %d: negative delay %v", i, got)
		}
	}
}

func TestExponentialBackOff_ZeroBase(t *testing.T) {
	fn := ExponentialBackOff(0, time.Minute, 0)
	for _, attempt := range []uint64{0, 1, 5} {
		if got := fn(attempt); got != 0 {
			t.Errorf("attempt %d: got %v, want 0", attempt, got)
		}
	}
}

func TestExponentialBackOff_AttemptZero(t *testing.T) {
	fn := ExponentialBackOff(time.Second, time.Minute, 0)
	got := fn(0)
	// attempt 0 maps to retry index 0 → base delay
	if got != time.Second {
		t.Errorf("attempt 0: got %v, want %v", got, time.Second)
	}
}
