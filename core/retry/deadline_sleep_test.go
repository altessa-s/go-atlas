// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/retry"
)

// fixedDelay retries after d whatever the attempt.
func fixedDelay(d time.Duration) retry.Option {
	return retry.WithNextDelay(func(int, error) time.Duration { return d })
}

func TestWithSleep(t *testing.T) {
	t.Parallel()

	t.Run("drives the backoff", func(t *testing.T) {
		t.Parallel()

		var slept []time.Duration
		calls := 0
		err := retry.Do(t.Context(), func(context.Context) error {
			calls++
			if calls < 3 {
				return errors.New("transient")
			}
			return nil
		},
			retry.WithMaxAttempts(5),
			retry.WithNextDelay(func(attempt int, _ error) time.Duration { return time.Duration(attempt+1) * time.Hour }),
			retry.WithSleep(func(_ context.Context, d time.Duration) error {
				slept = append(slept, d)
				return nil
			}),
		)
		require.NoError(t, err)
		require.Equal(t, 3, calls)
		require.Equal(t, []time.Duration{time.Hour, 2 * time.Hour}, slept, "no real sleep, delays as computed")
	})

	t.Run("its error ends Do", func(t *testing.T) {
		t.Parallel()

		errSleep := errors.New("sleep interrupted")
		calls := 0
		err := retry.Do(t.Context(), func(context.Context) error {
			calls++
			return errors.New("transient")
		},
			retry.WithMaxAttempts(5), fixedDelay(time.Hour),
			retry.WithSleep(func(context.Context, time.Duration) error { return errSleep }),
		)
		require.ErrorIs(t, err, errSleep)
		require.Equal(t, 1, calls)
	})
}

func TestWithStopBeforeDeadline(t *testing.T) {
	t.Parallel()

	errAttempt := errors.New("provider busy")

	t.Run("returns the last error instead of sleeping into the deadline", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		calls, retries := 0, 0
		err := retry.Do(ctx, func(context.Context) error {
			calls++
			return errAttempt
		},
			retry.WithMaxAttempts(5), fixedDelay(time.Hour), retry.WithStopBeforeDeadline(),
			retry.WithOnRetry(func(int, error, time.Duration) { retries++ }),
		)
		require.ErrorIs(t, err, errAttempt)
		require.NotErrorIs(t, err, context.DeadlineExceeded)
		require.Equal(t, 1, calls)
		require.Zero(t, retries, "no retry is announced")
	})

	t.Run("retries whose delay ends before the deadline run", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		calls := 0
		err := retry.Do(ctx, func(context.Context) error {
			calls++
			if calls < 3 {
				return errAttempt
			}
			return nil
		},
			retry.WithMaxAttempts(5), fixedDelay(time.Millisecond), retry.WithStopBeforeDeadline(),
		)
		require.NoError(t, err)
		require.Equal(t, 3, calls)
	})

	t.Run("no deadline: no effect", func(t *testing.T) {
		t.Parallel()

		calls := 0
		err := retry.Do(context.WithoutCancel(t.Context()), func(context.Context) error {
			calls++
			return errAttempt
		},
			retry.WithMaxAttempts(2), fixedDelay(time.Hour), retry.WithStopBeforeDeadline(),
			retry.WithSleep(func(context.Context, time.Duration) error { return nil }),
		)
		require.ErrorIs(t, err, errAttempt)
		require.Equal(t, 3, calls)
	})

	t.Run("without the option the deadline ends the sleep", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()
		err := retry.Do(ctx, func(context.Context) error { return errAttempt },
			retry.WithMaxAttempts(5), fixedDelay(time.Hour))
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

// retryAfterError is a failure that asks for a minimum pause.
type retryAfterError struct{ after time.Duration }

func (retryAfterError) Error() string { return "busy" }

// TestNextDelayRaisedFromError shows the Retry-After pattern: the delay
// callback receives the failed attempt's error and raises the backoff to the
// minimum the error carries.
func TestNextDelayRaisedFromError(t *testing.T) {
	t.Parallel()

	backoff := retry.Exponential(retry.ExponentialConfig{BaseDelay: time.Millisecond, Factor: 1})
	var slept []time.Duration
	calls := 0
	err := retry.Do(t.Context(), func(context.Context) error {
		calls++
		if calls == 1 {
			return retryAfterError{after: 3 * time.Second}
		}
		return nil
	},
		retry.WithMaxAttempts(1),
		retry.WithNextDelay(func(attempt int, err error) time.Duration {
			d := backoff(attempt, err)
			if ra, ok := errors.AsType[retryAfterError](err); ok {
				d = max(d, ra.after)
			}
			return d
		}),
		retry.WithSleep(func(_ context.Context, d time.Duration) error {
			slept = append(slept, d)
			return nil
		}),
	)
	require.NoError(t, err)
	require.Equal(t, []time.Duration{3 * time.Second}, slept)
}
