// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clock_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/core/time/clock"
)

var benchTime time.Time

func BenchmarkRealNow(b *testing.B) {
	c := clock.Real()
	for b.Loop() {
		benchTime = c.Now()
	}
}

func BenchmarkFakeNow(b *testing.B) {
	c := clock.NewFake(t0)
	for b.Loop() {
		benchTime = c.Now()
	}
}
