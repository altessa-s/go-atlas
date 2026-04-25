// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package helpers_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/helpers"
)

func BenchmarkGoroutineID(b *testing.B) {
	var sink int
	for b.Loop() {
		sink = helpers.GoroutineID()
	}
	_ = sink
}
