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

func TestDo(t *testing.T) {
	t.Run("SuccessFirstTry", func(t *testing.T) {
		calls := 0
		err := retry.Do(t.Context(), func(ctx context.Context) error {
			calls++
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, 1, calls)
	})

	t.Run("RetryThenSuccess", func(t *testing.T) {
		calls := 0
		err := retry.Do(t.Context(), func(ctx context.Context) error {
			calls++
			if calls < 3 {
				return errors.New("fail")
			}
			return nil
		},
			retry.WithMaxAttempts(3),
			retry.WithNextDelay(func(int, error) time.Duration { return time.Nanosecond }),
		)
		require.NoError(t, err)
		require.Equal(t, 3, calls)
	})

	t.Run("MaxAttemptsExceeded", func(t *testing.T) {
		calls := 0
		failErr := errors.New("fail")
		err := retry.Do(t.Context(), func(ctx context.Context) error {
			calls++
			return failErr
		},
			retry.WithMaxAttempts(2), // 3 attempts total (0, 1, 2)
			retry.WithNextDelay(func(int, error) time.Duration { return time.Nanosecond }),
		)
		require.Equal(t, failErr, err)
		require.Equal(t, 3, calls)
	})

	t.Run("ShouldRetryStops", func(t *testing.T) {
		calls := 0
		stopErr := errors.New("stop")
		err := retry.Do(t.Context(), func(ctx context.Context) error {
			calls++
			return stopErr
		},
			retry.WithMaxAttempts(5),
			retry.WithShouldRetry(func(err error) bool { return err != stopErr }),
			retry.WithNextDelay(func(int, error) time.Duration { return time.Nanosecond }),
		)
		require.Equal(t, stopErr, err)
		require.Equal(t, 1, calls)
	})

	t.Run("MaxElapsedTime", func(t *testing.T) {
		err := retry.Do(t.Context(), func(ctx context.Context) error {
			return errors.New("fail")
		},
			retry.WithMaxAttempts(100),
			retry.WithMaxElapsedTime(time.Millisecond*10),
			retry.WithNextDelay(func(int, error) time.Duration { return time.Millisecond * 5 }),
		)
		require.Error(t, err, "should have failed due to timeout")
	})

	t.Run("ContextCancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		calls := 0
		err := retry.Do(ctx, func(ctx context.Context) error {
			calls++
			return errors.New("fail")
		},
			retry.WithMaxAttempts(5),
			retry.WithNextDelay(func(int, error) time.Duration {
				cancel() // Cancel on first retry attempt check
				return time.Millisecond
			}),
		)

		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestExponential(t *testing.T) {
	tests := []struct {
		name    string
		config  retry.ExponentialConfig
		attempt int
		minDur  time.Duration
		maxDur  time.Duration
	}{
		{
			name:    "Base",
			config:  retry.ExponentialConfig{BaseDelay: time.Second, Factor: 2},
			attempt: 0,
			minDur:  time.Second,
			maxDur:  time.Second,
		},
		{
			name:    "SecondAttempt",
			config:  retry.ExponentialConfig{BaseDelay: time.Second, Factor: 2},
			attempt: 1,
			minDur:  time.Second * 2,
			maxDur:  time.Second * 2,
		},
		{
			name:    "Cap",
			config:  retry.ExponentialConfig{BaseDelay: time.Second, Factor: 2, MaxDelay: time.Second * 3},
			attempt: 2, // 4s -> capped at 3s
			minDur:  time.Second * 3,
			maxDur:  time.Second * 3,
		},
		{
			// Regression: for large attempts, math.Pow overflows to +Inf
			// and the float64->int64 cast yields MinInt64, which the
			// internal "d <= 0" guard used to collapse to a zero delay
			// (defeating MaxDelay and causing callers to hot-loop).
			name:    "OverflowCapsToMaxDelay",
			config:  retry.ExponentialConfig{BaseDelay: time.Millisecond, Factor: 2, MaxDelay: 30 * time.Second},
			attempt: 128,
			minDur:  30 * time.Second,
			maxDur:  30 * time.Second,
		},
		{
			// Same overflow path but with MaxDelay unset: the function
			// should return a finite, large duration rather than zero.
			name:    "OverflowUncappedReturnsMaxInt64",
			config:  retry.ExponentialConfig{BaseDelay: time.Millisecond, Factor: 2},
			attempt: 128,
			minDur:  time.Hour,
			maxDur:  time.Duration(1<<63 - 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := retry.Exponential(tt.config)
			got := fn(tt.attempt, nil)
			require.True(t, got >= tt.minDur && got <= tt.maxDur,
				"Exponential() = %v, want within [%v, %v]", got, tt.minDur, tt.maxDur)
		})
	}
}

func TestExponential_Jitter(t *testing.T) {
	t.Run("ZeroJitter_Deterministic", func(t *testing.T) {
		fn := retry.Exponential(retry.ExponentialConfig{BaseDelay: time.Second, Factor: 2, Jitter: 0})
		a := fn(1, nil)
		b := fn(1, nil)
		require.Equal(t, a, b, "zero jitter should be deterministic")
		require.Equal(t, 2*time.Second, a)
	})

	t.Run("WithJitter_Bounds", func(t *testing.T) {
		base := time.Second
		fn := retry.Exponential(retry.ExponentialConfig{BaseDelay: base, Factor: 1, Jitter: 0.5})
		for range 100 {
			d := fn(0, nil)
			require.GreaterOrEqual(t, d, base, "delay %v < base %v", d, base)
			// max = base + base*0.5 = 1.5s
			require.LessOrEqual(t, d, base+base/2, "delay %v > max jitter bound 1.5s", d)
		}
	})

	t.Run("WithJitter_NonDeterministic", func(t *testing.T) {
		fn := retry.Exponential(retry.ExponentialConfig{BaseDelay: time.Second, Factor: 2, Jitter: 0.5})
		seen := make(map[time.Duration]struct{})
		for range 100 {
			seen[fn(3, nil)] = struct{}{}
		}
		require.GreaterOrEqual(t, len(seen), 2, "jitter should produce varying delays")
	})

	t.Run("JitterCappedAtMaxDelay", func(t *testing.T) {
		maxDelay := 3 * time.Second
		fn := retry.Exponential(retry.ExponentialConfig{
			BaseDelay: time.Second,
			Factor:    2,
			MaxDelay:  maxDelay,
			Jitter:    1.0,
		})
		for range 100 {
			d := fn(10, nil) // base * 2^10 = 1024s, capped to 3s, then +jitter re-capped
			require.LessOrEqual(t, d, maxDelay, "delay %v > MaxDelay %v", d, maxDelay)
		}
	})

	t.Run("JitterAboveOneClamped", func(t *testing.T) {
		base := time.Second
		fn := retry.Exponential(retry.ExponentialConfig{BaseDelay: base, Factor: 1, Jitter: 5.0})
		for range 100 {
			d := fn(0, nil)
			// clamped to 1.0 → max = base + base*1.0 = 2s
			require.LessOrEqual(t, d, 2*base, "delay %v exceeds 2x base (jitter should clamp to 1.0)", d)
		}
	})
}

func TestExponentialConfigPool(t *testing.T) {
	cfg := retry.GetExponentialConfig()
	require.NotNil(t, cfg)
	retry.PutExponentialConfig(cfg)
}

func TestPolicy(t *testing.T) {
	t.Run("ZeroOptionsSingleAttempt", func(t *testing.T) {
		p := retry.NewPolicy()
		calls := 0
		wantErr := errors.New("fail")
		err := p.Do(t.Context(), func(ctx context.Context) error {
			calls++
			return wantErr
		})
		require.ErrorIs(t, err, wantErr)
		require.Equal(t, 1, calls)
	})

	t.Run("RetryThenSuccess", func(t *testing.T) {
		p := retry.NewPolicy(
			retry.WithMaxAttempts(3),
			retry.WithNextDelay(func(int, error) time.Duration { return time.Nanosecond }),
		)
		calls := 0
		err := p.Do(t.Context(), func(ctx context.Context) error {
			calls++
			if calls < 3 {
				return errors.New("fail")
			}
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, 3, calls)
	})

	t.Run("ReusableAcrossCalls", func(t *testing.T) {
		p := retry.NewPolicy(
			retry.WithMaxAttempts(2),
			retry.WithNextDelay(func(int, error) time.Duration { return time.Nanosecond }),
		)
		wantErr := errors.New("fail")
		for range 2 {
			calls := 0
			err := p.Do(t.Context(), func(ctx context.Context) error {
				calls++
				return wantErr
			})
			require.ErrorIs(t, err, wantErr)
			require.Equal(t, 3, calls, "each run must get the full attempt budget")
		}
	})
}
