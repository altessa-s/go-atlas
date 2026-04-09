// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nonewprivs_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/nonewprivs"
)

// Set is irreversible for the lifetime of the process, so it cannot be
// benchmarked in-loop. Enabled is a single prctl call and is the only
// pure-read entry point worth measuring.
func BenchmarkEnabled(b *testing.B) {
	var sink bool
	for b.Loop() {
		sink, _ = nonewprivs.Enabled()
	}
	_ = sink
}
