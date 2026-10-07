// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package claimscope_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/auth/internal/claimscope"
)

func BenchmarkSeq(b *testing.B) {
	inputs := map[string]any{
		"string": "files:read files:write users:read admin",
		"any":    []any{"files:read", "files:write", "users:read", "admin"},
	}
	for name, in := range inputs {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				for range claimscope.Seq(in) { //nolint:revive // drain the iterator
				}
			}
		})
	}
}
