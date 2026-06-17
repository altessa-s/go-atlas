// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/config"

	sagafactory "github.com/altessa-s/go-atlas/data/saga/factory"
)

func BenchmarkBuild(b *testing.B) {
	cfg := config.DefaultSaga()
	def := orderDef()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := sagafactory.New(&cfg, def).Build(); err != nil {
			b.Fatal(err)
		}
	}
}
