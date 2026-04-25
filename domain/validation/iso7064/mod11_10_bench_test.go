// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package iso7064_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/domain/validation/iso7064"
)

func BenchmarkMod11_10_String(b *testing.B) {
	input := "123456788"
	b.ResetTimer()
	for b.Loop() {
		_, _ = iso7064.Mod11_10(input)
	}
}

func BenchmarkMod11_10_Int64(b *testing.B) {
	input := int64(123456788)
	b.ResetTimer()
	for b.Loop() {
		_, _ = iso7064.Mod11_10(input)
	}
}

func BenchmarkIsValidMod11_10(b *testing.B) {
	input := "123456788"
	b.ResetTimer()
	for b.Loop() {
		_ = iso7064.IsValidMod11_10(input)
	}
}
