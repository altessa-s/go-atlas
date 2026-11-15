// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"context"
	"time"

	"github.com/altessa-s/go-atlas-core/errors/network"

	coreretry "github.com/altessa-s/go-atlas-core/retry"
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

	// retryBaseDelay is the first backoff delay, matching the
	// cenkalti/backoff default.
	retryBaseDelay = 500 * time.Millisecond
)

// Retry executes the given function with exponential backoff retry logic.
// It retries on transient network errors (timeouts, connection refused, network errors)
// and returns immediately on permanent errors. Without options it makes up to
// [DefaultMaxRetries] attempts within [DefaultMaxElapsedTime], spreading the
// delays by [DefaultJitter].
//
// Example:
//
//	entry, err := Retry(ctx, func() (jetstream.KeyValueEntry, error) {
//	    return kv.Get(ctx, key)
//	})
func Retry[T any](ctx context.Context, fn func() (T, error), opts ...RetryOption) (T, error) {
	var last T

	cfg := newRetryOptions(opts...)

	maxAttempts := 0
	if cfg.maxAttempts > 0 {
		maxAttempts = int(cfg.maxAttempts) - 1 //nolint:gosec // G115: maxAttempts is bounded by config validation
	}

	err := coreretry.Do(ctx, func(context.Context) error {
		res, err := fn()
		last = res
		return err
	},
		coreretry.WithMaxAttempts(maxAttempts),
		coreretry.WithMaxElapsedTime(cfg.maxElapsedTime),
		coreretry.WithShouldRetry(isTransientError),
		coreretry.WithNextDelay(coreretry.Exponential(
			retryBaseDelay,
			coreretry.WithExponentialJitter(cfg.jitter),
		)),
	)

	return last, err
}

// isTransientError returns true if the error is a transient network error
// that should be retried.
func isTransientError(err error) bool {
	return network.IsRequestTimeoutError(err) ||
		network.IsNetworkError(err) ||
		network.IsConnectionRefused(err)
}
