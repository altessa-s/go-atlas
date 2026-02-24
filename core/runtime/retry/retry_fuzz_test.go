// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retry_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/retry"
)

func FuzzExponential(f *testing.F) {
	f.Add(int64(time.Second), int64(time.Minute), 2.0, 5, 0.0)  // deterministic
	f.Add(int64(time.Second), int64(time.Minute), 2.0, 5, 0.25) // with jitter
	f.Add(int64(time.Second), int64(time.Minute), 1.5, 0, 1.0)  // max jitter
	f.Add(int64(time.Second), int64(time.Minute), 2.0, 10, 5.0) // jitter > 1 (clamped)

	f.Fuzz(func(t *testing.T, base int64, maxDelay int64, factor float64, attempt int, jitter float64) {
		cfg := retry.ExponentialConfig{
			BaseDelay: time.Duration(base),
			MaxDelay:  time.Duration(maxDelay),
			Factor:    factor,
			Jitter:    jitter,
		}

		fn := retry.Exponential(cfg)
		got := fn(attempt, nil)

		if got < 0 {
			t.Errorf("Exponential returned negative duration: %v", got)
		}

		if maxDelay > 0 && time.Duration(maxDelay) > 0 {
			if got > time.Duration(maxDelay) {
				t.Errorf("Exponential returned duration > MaxDelay: %v > %v", got, maxDelay)
			}
		}
	})
}
