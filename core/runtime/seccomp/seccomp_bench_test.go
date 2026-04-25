// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package seccomp_test

import (
	"errors"
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/seccomp"
)

// BlockDangerousSyscalls is irreversible and cannot be benchmarked
// in-loop — seccomp filters form an append-only stack. The benchmark
// below instead exercises the sentinel error comparison path that
// callers use to branch on platform support.
func BenchmarkErrUnsupportedIs(b *testing.B) {
	err := errors.New("wrapped: " + seccomp.ErrUnsupported.Error())
	var sink bool
	for b.Loop() {
		sink = errors.Is(err, seccomp.ErrUnsupported)
	}
	_ = sink
}
