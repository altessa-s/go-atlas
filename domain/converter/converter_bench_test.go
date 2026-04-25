// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/domain/converter"
)

type BenchSmall struct {
	A, B, C int
}

type BenchLarge struct {
	A, B, C, D, E, F, G, H string
	I, J, K, L, M, N, O, P int
	Nested                 BenchSmall
}

func BenchmarkConvert_Small(b *testing.B) {
	src := BenchSmall{A: 1, B: 2, C: 3}
	var dst BenchSmall

	// Create converter once to amortize setup
	conv := converter.New[BenchSmall, *BenchSmall]()

	b.ResetTimer()
	for b.Loop() {
		conv.Convert(src, &dst)
	}
}

func BenchmarkConvert_Large(b *testing.B) {
	src := BenchLarge{
		A: "a", B: "b", C: "c", D: "d", E: "e", F: "f", G: "g", H: "h",
		I: 1, J: 2, K: 3, L: 4, M: 5, N: 6, O: 7, P: 8,
		Nested: BenchSmall{A: 1, B: 2, C: 3},
	}
	var dst BenchLarge

	conv := converter.New[BenchLarge, *BenchLarge]()

	b.ResetTimer()
	for b.Loop() {
		conv.Convert(src, &dst)
	}
}

func BenchmarkConvert_Slice(b *testing.B) {
	src := make([]BenchSmall, 1000)
	for i := range src {
		src[i] = BenchSmall{A: i, B: i, C: i}
	}
	var dst []BenchSmall

	conv := converter.New[[]BenchSmall, *[]BenchSmall]()

	b.ResetTimer()
	for b.Loop() {
		conv.Convert(src, &dst)
	}
}

func BenchmarkConvert_OneOff(b *testing.B) {
	// Tests overhead of creating new converter every time (New + Convert)
	src := BenchSmall{A: 1, B: 2, C: 3}
	var dst BenchSmall

	b.ResetTimer()
	for b.Loop() {
		converter.Convert(src, &dst)
	}
}
