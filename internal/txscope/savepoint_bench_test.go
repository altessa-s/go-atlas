// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package txscope_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/internal/txscope"
)

func BenchmarkOpenWithin(b *testing.B) {
	outer, finish := txscope.Begin(b.Context())
	defer finish(false)
	inner, end := txscope.Begin(outer)
	defer end(false)
	scope := txscope.Current(inner)
	for b.Loop() {
		scope.OpenWithin(nil)
	}
}
