// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package landlock_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/landlock"
)

// Apply mutates the current process's Landlock ruleset and cannot be
// safely benchmarked in-loop. The benchmarks below exercise the option
// construction path only, which is the hot path callers actually touch
// from their startup code.

func BenchmarkWithReadPaths(b *testing.B) {
	var sink landlock.Option
	for b.Loop() {
		sink = landlock.WithReadPaths("/etc", "/usr/share")
	}
	_ = sink
}

func BenchmarkWithReadWritePaths(b *testing.B) {
	var sink landlock.Option
	for b.Loop() {
		sink = landlock.WithReadWritePaths("/var/lib/app", "/var/log/app")
	}
	_ = sink
}
