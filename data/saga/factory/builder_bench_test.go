// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
	sagafactory "github.com/altessa-s/go-atlas/data/saga/factory"
)

func BenchmarkBuild(b *testing.B) {
	cfg := sagaconfig.Default()
	def := orderDef()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := sagafactory.New(&cfg, def).Build(); err != nil {
			b.Fatal(err)
		}
	}
}
