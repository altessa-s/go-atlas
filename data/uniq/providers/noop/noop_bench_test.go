// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package noop_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/uniq/providers/noop"
)

func BenchmarkProvider_Add(b *testing.B) {
	p := noop.New()
	ctx := b.Context()
	for b.Loop() {
		_ = p.Add(ctx, "key")
	}
}

func BenchmarkProvider_Exist(b *testing.B) {
	p := noop.New()
	ctx := b.Context()
	for b.Loop() {
		_, _ = p.Exist(ctx, "key")
	}
}
