// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package regexanchor_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/filter/internal/regexanchor"
)

func BenchmarkEndOfText(b *testing.B) {
	for b.Loop() {
		_, _ = regexanchor.EndOfText(`^(?P<user>[a-z0-9._-]+)@(?:[a-z0-9-]+\.)+[a-z]{2,}$`)
	}
}

func BenchmarkEndOfText_NoDollar(b *testing.B) {
	for b.Loop() {
		_, _ = regexanchor.EndOfText(`^(?P<user>[a-z0-9._-]+)@(?:[a-z0-9-]+\.)+[a-z]{2,}`)
	}
}
