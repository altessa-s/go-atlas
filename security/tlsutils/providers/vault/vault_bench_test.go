// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsvault_test

import (
	"testing"

	tlsvault "github.com/altessa-s/go-atlas/security/tlsutils/providers/vault"
)

func BenchmarkRSAGenerator_Generate(b *testing.B) {
	gen := tlsvault.NewRSAGenerator()
	// First call generates, subsequent calls use cache
	_, _ = gen.Generate()
	b.ResetTimer()
	for b.Loop() {
		_, _ = gen.Generate()
	}
}

func BenchmarkNewLogger(b *testing.B) {
	for b.Loop() {
		_ = tlsvault.NewLogger(nil)
	}
}
