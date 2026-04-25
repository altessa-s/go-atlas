// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retry

import (
	"cmp"
	"context"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	coretime "github.com/altessa-s/go-atlas/core/time"
)

// defaultExponentialFactor is the default exponential backoff multiplier per attempt.
const defaultExponentialFactor = 1.5

var exponentialConfigPool = sync.Pool{
	New: func() any {
		return &ExponentialConfig{}
	},
}

// GetExponentialConfig returns a reset [ExponentialConfig] from an internal
// sync.Pool. Call [PutExponentialConfig] when the config is no longer needed
// to return it for reuse, reducing allocation pressure in hot loops.
func GetExponentialConfig() *ExponentialConfig {
	config, ok := exponentialConfigPool.Get().(*ExponentialConfig)
	if !ok {
		return &ExponentialConfig{}
	}
	*config = ExponentialConfig{} // Reset to defaults
	return config
}

// PutExponentialConfig resets config and returns it to the internal sync.Pool
// for reuse. Passing nil is a safe no-op.
func PutExponentialConfig(config *ExponentialConfig) {
	if config == nil {
		return
	}
	*config = ExponentialConfig{} // Reset before putting back
	exponentialConfigPool.Put(config)
}

// Do calls fn repeatedly according to the policy supplied via opts until
// one of the following occurs:
//   - fn returns nil (success) — Do returns nil.
//   - ctx is canceled — Do returns ctx.Err().
//   - maxAttempts is exhausted — Do returns the last error from fn.
//   - maxElapsedTime is exceeded — Do returns the last error from fn.
//   - The shouldRetry filter returns false — Do returns the error immediately.
//   - The nextDelay callback is nil or returns ≤ 0 — Do returns the error
//     immediately.
//
// A nil ctx is silently replaced with [context.Background]. Without any
// options the call behaves as a single attempt (no retries): pass
// [WithMaxAttempts] and [WithNextDelay] to enable retrying.
//
//nolint:contextcheck // Do treats nil ctx as Background for convenience (callers should pass an inherited context).
func Do(ctx context.Context, fn func(context.Context) error, opts ...Option) error {
	cfg := newOptions(opts...)
	ctx = corecontext.OrBackground(ctx)

	start := time.Now()

	var (
		timer   *time.Timer
		lastErr error
	)
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for attempt := 0; cfg.maxAttempts < 0 || attempt <= cfg.maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := fn(ctx)
		if err == nil {
			return nil
		}
		lastErr = err

		if cfg.shouldRetry != nil && !cfg.shouldRetry(err) {
			return err
		}

		if cfg.nextDelay == nil {
			return err
		}

		delay := cfg.nextDelay(attempt, err)
		if delay <= 0 {
			return err
		}

		if cfg.maxElapsedTime > 0 {
			elapsed := time.Since(start)
			if elapsed >= cfg.maxElapsedTime {
				return err
			}
			if elapsed+delay > cfg.maxElapsedTime {
				return err
			}
		}

		if cfg.onRetry != nil {
			cfg.onRetry(attempt, err, delay)
		}

		// Sleep with a reusable timer to avoid time.After allocations in a loop.
		if timer == nil {
			timer = time.NewTimer(delay)
		} else {
			coretime.TimerStopAndDrain(timer)
			timer.Reset(delay)
		}

		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return lastErr
}

// ExponentialConfig holds the parameters for [Exponential]. It is a
// plain value type rather than a functional-options struct because it
// is used both as an argument to [Exponential] and as a pooled value via
// [GetExponentialConfig] / [PutExponentialConfig] in hot loops.
type ExponentialConfig struct {
	// BaseDelay is the delay applied on the first retry (attempt 0).
	// A zero or negative value causes [Exponential] to return 0 for every attempt.
	BaseDelay time.Duration

	// MaxDelay caps the computed delay. A zero value means the delay is unbounded.
	MaxDelay time.Duration

	// Factor is the exponential multiplier applied per attempt
	// (delay = BaseDelay * Factor^attempt). A zero value defaults to 1.5.
	Factor float64

	// Jitter is the maximum fraction of the computed delay to add as
	// randomized jitter (0.0–1.0). Values above 1.0 are clamped to 1.0.
	// A zero value (the default) produces deterministic delays with no jitter.
	Jitter float64
}

// Exponential returns a [NextDelayFunc] that implements exponential
// backoff with optional jitter, suitable for [WithNextDelay]:
//
//	delay(attempt) = min(MaxDelay, BaseDelay * Factor^attempt + jitter)
//
// When [ExponentialConfig.Jitter] is zero the delays are deterministic.
// The returned function is safe for concurrent use.
func Exponential(cfg ExponentialConfig) NextDelayFunc {
	factor := cmp.Or(cfg.Factor, defaultExponentialFactor)
	jitter := min(cfg.Jitter, 1.0)

	return func(attempt int, _ error) time.Duration {
		if cfg.BaseDelay <= 0 {
			return 0
		}
		if attempt < 0 {
			attempt = 0
		}

		// math.Pow returns +Inf for large attempt counts (e.g. factor=2,
		// attempt=64). The resulting product overflows the float64→int64
		// conversion and yields math.MinInt64 on current Go runtimes —
		// which the "d <= 0" guard below would silently turn into a
		// zero-delay hot loop, defeating MaxDelay entirely. Detect the
		// overflow up front and clamp to MaxDelay (or the maximum
		// representable duration when MaxDelay is uncapped).
		multiplier := math.Pow(factor, float64(attempt))
		product := float64(cfg.BaseDelay) * multiplier
		if math.IsInf(multiplier, 1) || product >= float64(math.MaxInt64) {
			if cfg.MaxDelay > 0 {
				return cfg.MaxDelay
			}
			return time.Duration(math.MaxInt64)
		}

		d := time.Duration(product)
		if d <= 0 {
			return 0
		}
		if cfg.MaxDelay > 0 && d > cfg.MaxDelay {
			d = cfg.MaxDelay
		}

		if jitter > 0 {
			d += time.Duration(float64(d) * jitter * rand.Float64())
			if cfg.MaxDelay > 0 && d > cfg.MaxDelay {
				d = cfg.MaxDelay
			}
		}

		return d
	}
}
