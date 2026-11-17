// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	retryqueueconfig "github.com/altessa-s/go-atlas/config/retryqueue"
	retryqueuefactory "github.com/altessa-s/go-atlas/factory/reliability/retryqueue"
)

func BenchmarkBuild(b *testing.B) {
	cfg := retryqueueconfig.Default()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := retryqueuefactory.New[payload](&cfg).Build(b.Context()); err != nil {
			b.Fatal(err)
		}
	}
}
