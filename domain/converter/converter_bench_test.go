// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter_test

import (
	"reflect"
	"testing"

	"github.com/altessa-s/go-atlas/domain/converter"

	convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
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

// BenchmarkConvert_Large_WithPassthroughCodec is the baseline for the
// codec-registered path. PR #46 reroutes the entire dispatch through
// the codec chain when any codec is registered (so a codec-handled
// struct type doesn't get shadowed by the built-in field-by-field
// copy). The passthrough codec used here never handles anything — it
// always delegates to the terminal handler — so this bench measures
// the per-field overhead of the chain-routing path against the
// codec-free baseline ([BenchmarkConvert_Large]). The two should be
// roughly comparable; a large regression points at the codec
// indirection (or the convertByKindHandler caching going stale).
func BenchmarkConvert_Large_WithPassthroughCodec(b *testing.B) {
	src := BenchLarge{
		A: "a", B: "b", C: "c", D: "d", E: "e", F: "f", G: "g", H: "h",
		I: 1, J: 2, K: 3, L: 4, M: 5, N: 6, O: 7, P: 8,
		Nested: BenchSmall{A: 1, B: 2, C: 3},
	}
	var dst BenchLarge

	passthrough := func(field string, s, d reflect.Value, next convcodec.CodecHandler) {
		next(field, s, d)
	}

	conv := converter.New[BenchLarge, *BenchLarge](converter.WithCodecs(passthrough))

	b.ResetTimer()
	for b.Loop() {
		conv.Convert(src, &dst)
	}
}

// BenchmarkConvert_SparseMerge measures the sparse-merge path: a
// sparse pointer-based source applied onto a populated destination, with a
// nested struct merged recursively.
func BenchmarkConvert_SparseMerge(b *testing.B) {
	name := "name"
	field := "field"

	type benchMergeNested struct {
		Field *string
		Label *string
	}
	type benchMerge struct {
		Name   *string
		Title  *string
		Nested *benchMergeNested
	}

	src := benchMerge{Name: &name, Nested: &benchMergeNested{Field: &field}}
	dst := benchMerge{Title: &name, Nested: &benchMergeNested{Label: &field}}

	conv := converter.New[*benchMerge, *benchMerge](converter.WithSparseMerge())

	b.ResetTimer()
	for b.Loop() {
		conv.Convert(&src, &dst)
	}
}
