// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import (
	"os"
	"testing"
	"time"
)

// StressDurationEnv names the environment variable that sets how long each
// stress test (build tag `stress`) runs, as a Go duration such as "10s".
const StressDurationEnv = "ATLAS_STRESS_DURATION"

// DefaultStressDuration is the per-test stress duration when
// [StressDurationEnv] is unset.
const DefaultStressDuration = 30 * time.Second

// StressDuration returns how long a stress test should run: the value of
// [StressDurationEnv], or [DefaultStressDuration] when it is unset. A value
// that does not parse as a positive duration fails the test.
func StressDuration(tb testing.TB) time.Duration {
	tb.Helper()
	v := os.Getenv(StressDurationEnv)
	if v == "" {
		return DefaultStressDuration
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		tb.Fatalf("%s=%q: want a positive Go duration such as 10s", StressDurationEnv, v)
	}
	return d
}
