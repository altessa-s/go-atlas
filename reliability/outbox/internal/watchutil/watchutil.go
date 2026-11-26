// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package watchutil

import (
	"context"
	"time"

	coreretry "github.com/altessa-s/go-atlas-core/retry"
	coreclock "github.com/altessa-s/go-atlas-core/time/clock"
)

// Default reconnect timings and log cadence shared by storage watchers.
const (
	// ReconnectBaseDelay is the pause before the first reconnect attempt.
	ReconnectBaseDelay = time.Second

	// ReconnectMaxDelay caps the pause between reconnect attempts.
	ReconnectMaxDelay = 30 * time.Second

	// ReconnectFactor is the exponential multiplier applied per attempt.
	ReconnectFactor = 1.5

	// ReconnectJitter spreads reconnects across instances that lost the same backend.
	ReconnectJitter = 0.2

	// LogEvery spaces warnings during a run of consecutive failures: the first
	// failure is logged, then every LogEvery-th.
	LogEvery = 10
)

// Sleep waits for d using the system clock and reports false when ctx ended
// first. Non-positive durations return immediately without allocating a timer,
// still honoring an already-ended context.
func Sleep(ctx context.Context, d time.Duration) bool {
	return coreclock.Real().Sleep(ctx, d) == nil
}

// Backoff returns an exponential backoff policy configured with the supplied
// base delay, maximum delay, growth factor, and jitter.
func Backoff(baseDelay, maxDelay time.Duration, factor, jitter float64) coreretry.NextDelayFunc {
	return coreretry.Exponential(
		baseDelay,
		coreretry.WithExponentialMaxDelay(maxDelay),
		coreretry.WithExponentialFactor(factor),
		coreretry.WithExponentialJitter(jitter),
	)
}

// ReconnectBackoff returns the standard exponential reconnect backoff used by
// outbox storage watchers.
func ReconnectBackoff() coreretry.NextDelayFunc {
	return Backoff(ReconnectBaseDelay, ReconnectMaxDelay, ReconnectFactor, ReconnectJitter)
}

// ShouldLogFailure reports whether the n-th consecutive failure (1-indexed)
// should be logged: the first failure, then every [LogEvery]-th.
func ShouldLogFailure(n int) bool {
	return n > 0 && (n == 1 || n%LogEvery == 0)
}
