// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"context"
	"time"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreretry "github.com/altessa-s/go-atlas/core/retry"
)

const (
	// DefaultMaxRetries is the default maximum number of retry attempts.
	DefaultMaxRetries = 5

	// DefaultMaxElapsedTime is the default maximum elapsed time for retries.
	DefaultMaxElapsedTime = 2 * time.Second

	// DefaultJitter is the default randomization applied to each backoff
	// delay, as a fraction of that delay. Without it every node that hit the
	// same transient NATS failure retries on the same schedule and the
	// recovering server is hit by synchronized waves instead of a spread-out
	// trickle.
	DefaultJitter = 0.2
)

// RetryConfig holds configuration for retry operations.
type RetryConfig struct {
	MaxRetries     uint
	MaxElapsedTime time.Duration

	// Jitter is the maximum fraction of each computed delay added as
	// randomization (0.0–1.0). Zero produces deterministic, lockstep retries.
	Jitter float64
}

// DefaultRetryConfig returns the default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     DefaultMaxRetries,
		MaxElapsedTime: DefaultMaxElapsedTime,
		Jitter:         DefaultJitter,
	}
}

// Retry executes the given function with exponential backoff retry logic.
// It retries on transient network errors (timeouts, connection refused, network errors)
// and returns immediately on permanent errors.
//
// Example:
//
//	entry, err := Retry(ctx, func() (jetstream.KeyValueEntry, error) {
//	    return kv.Get(ctx, key)
//	})
func Retry[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	return RetryWithConfig(ctx, fn, DefaultRetryConfig())
}

// RetryWithConfig executes the given function with configurable retry logic.
func RetryWithConfig[T any](ctx context.Context, fn func() (T, error), cfg RetryConfig) (T, error) {
	var last T

	maxAttempts := 0
	if cfg.MaxRetries > 0 {
		maxAttempts = int(cfg.MaxRetries) - 1 //nolint:gosec // G115: MaxRetries is bounded by config validation
	}

	const baseDelay = 500 * time.Millisecond
	err := coreretry.Do(ctx, func(context.Context) error {
		res, err := fn()
		last = res
		return err
	},
		coreretry.WithMaxAttempts(maxAttempts),
		coreretry.WithMaxElapsedTime(cfg.MaxElapsedTime),
		coreretry.WithShouldRetry(isTransientError),
		coreretry.WithNextDelay(coreretry.Exponential(coreretry.ExponentialConfig{
			BaseDelay: baseDelay, // matches cenkalti/backoff default
			Jitter:    cfg.Jitter,
		})),
	)

	return last, err
}

// isTransientError returns true if the error is a transient network error
// that should be retried.
func isTransientError(err error) bool {
	return coreerrs.IsRequestTimeoutError(err) ||
		coreerrs.IsNetworkError(err) ||
		coreerrs.IsConnectionRefused(err)
}
