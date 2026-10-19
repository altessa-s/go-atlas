// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cas_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/mongo/cas"
)

func BenchmarkFilter(b *testing.B) {
	for b.Loop() {
		_ = cas.Filter("_id", "doc-1", "version", 42)
	}
}

func BenchmarkInc(b *testing.B) {
	for b.Loop() {
		_ = cas.Inc("version")
	}
}
