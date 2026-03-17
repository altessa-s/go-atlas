// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package msg

import (
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/retry"
)

// BackOffFunc computes the redelivery delay based on the current delivery attempt count.
// The attempt value equals the number of times the message has already been delivered
// (i.e. 1 on the first delivery, 2 on the second, etc.).
//
// Example — slice-based schedule:
//
//	func(attempt uint64) time.Duration {
//		delays := []time.Duration{5 * time.Second, 30 * time.Second, 5 * time.Minute}
//		return delays[min(int(attempt)-1, len(delays)-1)]
//	}
type BackOffFunc func(attempt uint64) time.Duration

// ScheduleBackOff returns a BackOffFunc that picks delays from a fixed schedule.
// After exhausting the schedule, the last delay repeats.
// Panics if no delays are provided.
//
// Example:
//
//	backoff := msg.ScheduleBackOff(5*time.Second, 30*time.Second, 5*time.Minute)
//	err := m.NakWithBackOff(backoff) // 5s, 30s, 5m, 5m, 5m, ...
func ScheduleBackOff(delays ...time.Duration) BackOffFunc {
	if len(delays) == 0 {
		panic("msg: ScheduleBackOff requires at least one delay")
	}

	// Copy to avoid caller mutation.
	schedule := make([]time.Duration, len(delays))
	copy(schedule, delays)

	return func(attempt uint64) time.Duration {
		if attempt == 0 {
			return schedule[0]
		}
		idx := int(attempt) - 1
		if idx >= len(schedule) {
			idx = len(schedule) - 1
		}
		return schedule[idx]
	}
}

// ExponentialBackOff returns a BackOffFunc with exponential growth and optional jitter.
// It delegates to [retry.Exponential] for the underlying math:
//
//	delay(attempt) = min(maxDelay, base * factor^(attempt-1) + jitter)
//
// factor defaults to 1.5 when zero. jitter is the maximum fraction of the
// computed delay to add as randomized noise (0.0–1.0).
//
// Example:
//
//	backoff := msg.ExponentialBackOff(time.Second, time.Minute, 0.1)
//	err := m.NakWithBackOff(backoff) // ~1s, ~1.5s, ~2.25s, ... capped at 1m
func ExponentialBackOff(base, maxDelay time.Duration, jitter float64) BackOffFunc {
	fn := retry.Exponential(retry.ExponentialConfig{
		BaseDelay: base,
		MaxDelay:  maxDelay,
		Jitter:    jitter,
	})

	return func(attempt uint64) time.Duration {
		// retry.Exponential is 0-indexed; attempt 1 (first delivery) maps to index 0.
		return fn(max(int(attempt)-1, 0), nil)
	}
}
