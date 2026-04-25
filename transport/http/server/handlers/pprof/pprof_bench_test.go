// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pprof_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/transport/http/server/handlers/pprof"
)

func BenchmarkMount(b *testing.B) {
	for b.Loop() {
		_ = pprof.Mount(&mockRouter{})
	}
}
