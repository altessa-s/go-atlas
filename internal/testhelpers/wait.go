// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import (
	"testing"
	"time"
)

// WaitFor polls condition every 5ms until it returns true or deadline expires.
// It calls tb.Fatal with msg if the deadline is exceeded, which stops the test
// immediately.
func WaitFor(tb testing.TB, deadline time.Duration, condition func() bool, msg string) {
	tb.Helper()
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-timer.C:
			tb.Fatal(msg)
		default:
			time.Sleep(5 * time.Millisecond) //nolint:mnd // poll interval
		}
	}
}
