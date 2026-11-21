// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package watermark_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/reliability/requestidstore/internal/watermark"
)

func BenchmarkObserve(b *testing.B) {
	var clock watermark.Clock
	now := time.Now()
	for b.Loop() {
		clock.Observe(now)
	}
}
