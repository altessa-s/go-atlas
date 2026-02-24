// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/retry"
)

func TestDo(t *testing.T) {
	t.Run("SuccessFirstTry", func(t *testing.T) {
		calls := 0
		err := retry.Do(t.Context(), retry.Config{}, func(ctx context.Context) error {
			calls++
			return nil
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if calls != 1 {
			t.Errorf("calls = %d, want 1", calls)
		}
	})

	t.Run("RetryThenSuccess", func(t *testing.T) {
		calls := 0
		err := retry.Do(t.Context(), retry.Config{
			MaxAttempts: 3,
			NextDelay:   func(int, error) time.Duration { return time.Nanosecond },
		}, func(ctx context.Context) error {
			calls++
			if calls < 3 {
				return errors.New("fail")
			}
			return nil
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if calls != 3 {
			t.Errorf("calls = %d, want 3", calls)
		}
	})

	t.Run("MaxAttemptsExceeded", func(t *testing.T) {
		calls := 0
		failErr := errors.New("fail")
		err := retry.Do(t.Context(), retry.Config{
			MaxAttempts: 2, // 3 attempts total (0, 1, 2)
			NextDelay:   func(int, error) time.Duration { return time.Nanosecond },
		}, func(ctx context.Context) error {
			calls++
			return failErr
		})
		if err != failErr {
			t.Errorf("error = %v, want %v", err, failErr)
		}
		if calls != 3 {
			t.Errorf("calls = %d, want 3", calls)
		}
	})

	t.Run("ShouldRetryStops", func(t *testing.T) {
		calls := 0
		stopErr := errors.New("stop")
		err := retry.Do(t.Context(), retry.Config{
			MaxAttempts: 5,
			ShouldRetry: func(err error) bool { return err != stopErr },
			NextDelay:   func(int, error) time.Duration { return time.Nanosecond },
		}, func(ctx context.Context) error {
			calls++
			return stopErr
		})
		if err != stopErr {
			t.Errorf("error = %v, want %v", err, stopErr)
		}
		if calls != 1 {
			t.Errorf("calls = %d, want 1", calls)
		}
	})

	t.Run("MaxElapsedTime", func(t *testing.T) {
		err := retry.Do(t.Context(), retry.Config{
			MaxAttempts:    100,
			MaxElapsedTime: time.Millisecond * 10,
			NextDelay:      func(int, error) time.Duration { return time.Millisecond * 5 },
		}, func(ctx context.Context) error {
			return errors.New("fail")
		})
		if err == nil {
			t.Error("should have failed due to timeout")
		}
	})

	t.Run("ContextCancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		calls := 0
		err := retry.Do(ctx, retry.Config{
			MaxAttempts: 5,
			NextDelay: func(int, error) time.Duration {
				cancel() // Cancel on first retry attempt check
				return time.Millisecond
			},
		}, func(ctx context.Context) error {
			calls++
			return errors.New("fail")
		})

		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want context.Canceled", err)
		}
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := retry.Exponential(tt.config)
			got := fn(tt.attempt, nil)
			if got < tt.minDur || got > tt.maxDur {
				t.Errorf("Exponential() = %v, want within [%v, %v]", got, tt.minDur, tt.maxDur)
			}
		})
	}
}

func TestExponential_Jitter(t *testing.T) {
	t.Run("ZeroJitter_Deterministic", func(t *testing.T) {
		fn := retry.Exponential(retry.ExponentialConfig{BaseDelay: time.Second, Factor: 2, Jitter: 0})
		a := fn(1, nil)
		b := fn(1, nil)
		if a != b {
			t.Errorf("zero jitter should be deterministic: %v != %v", a, b)
		}
		if a != 2*time.Second {
			t.Errorf("got %v, want 2s", a)
		}
	})

	t.Run("WithJitter_Bounds", func(t *testing.T) {
		base := time.Second
		fn := retry.Exponential(retry.ExponentialConfig{BaseDelay: base, Factor: 1, Jitter: 0.5})
		for range 100 {
			d := fn(0, nil)
			if d < base {
				t.Fatalf("delay %v < base %v", d, base)
			}
			// max = base + base*0.5 = 1.5s
			if d > base+base/2 {
				t.Fatalf("delay %v > max jitter bound 1.5s", d)
			}
		}
	})

	t.Run("WithJitter_NonDeterministic", func(t *testing.T) {
		fn := retry.Exponential(retry.ExponentialConfig{BaseDelay: time.Second, Factor: 2, Jitter: 0.5})
		seen := make(map[time.Duration]struct{})
		for range 100 {
			seen[fn(3, nil)] = struct{}{}
		}
		if len(seen) < 2 {
			t.Errorf("jitter should produce varying delays, got %d distinct values", len(seen))
		}
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
			if d > maxDelay {
				t.Fatalf("delay %v > MaxDelay %v", d, maxDelay)
			}
		}
	})

	t.Run("JitterAboveOneClamped", func(t *testing.T) {
		base := time.Second
		fn := retry.Exponential(retry.ExponentialConfig{BaseDelay: base, Factor: 1, Jitter: 5.0})
		for range 100 {
			d := fn(0, nil)
			// clamped to 1.0 → max = base + base*1.0 = 2s
			if d > 2*base {
				t.Fatalf("delay %v exceeds 2x base (jitter should clamp to 1.0)", d)
			}
		}
	})
}

func TestExponentialConfigPool(t *testing.T) {
	cfg := retry.GetExponentialConfig()
	if cfg == nil {
		t.Fatal("GetExponentialConfig returned nil")
	}
	retry.PutExponentialConfig(cfg)
}
