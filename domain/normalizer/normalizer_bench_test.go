// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package normalizer_test

import (
	"strconv"
	"testing"

	"github.com/altessa-s/go-atlas/domain/normalizer"
)

type BenchStruct struct {
	A string `normalize:"trim,lowercase"`
	B string `normalize:"trim"`
	C string `normalize:"lowercase"`
	D string `normalize:"trim,uppercase"`
}

type NestedBench struct {
	Name string `normalize:"trim"`
	Subs []BenchStruct
}

func BenchmarkNormalize_Simple(b *testing.B) {
	s := BenchStruct{
		A: "  TEST A  ",
		B: "  Test B  ",
		C: "Test C",
		D: "  test d  ",
	}

	b.ResetTimer()
	for b.Loop() {
		// We copy to simulate usage
		tmp := s
		_ = normalizer.Normalize(&tmp)
	}
}

func BenchmarkNormalize_Nested(b *testing.B) {
	nb := NestedBench{
		Name: "  Master  ",
		Subs: make([]BenchStruct, 100),
	}
	for i := range nb.Subs {
		nb.Subs[i] = BenchStruct{
			A: "  TEST " + strconv.Itoa(i),
			B: "  Test  ",
			C: "Test",
			D: "  test  ",
		}
	}

	b.ResetTimer()
	for b.Loop() {
		// Deep copy somewhat expensive so we might just re-normalize same struct
		// Normalizing already normalized struct should be faster (idempotent checks in modifiers)
		// But to be fair we should reset, but that kills benchmark.
		// The modifiers: trim, lowercase check 'already' state potentially?
		// basic_string_modifiers.go has `already` func for optimization.
		// So repetitive normalization benchmarks 'idempotent' path mostly.
		// To benchmark 'dirty' path we need fresh data.

		// Let's benchmark "Mixed" - reset one field
		nb.Name = "  Master  "
		_ = normalizer.Normalize(&nb)
	}
}
