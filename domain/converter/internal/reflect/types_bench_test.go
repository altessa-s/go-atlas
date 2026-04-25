// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package reflect_test

import (
	"reflect"
	"testing"

	reflectutils "github.com/altessa-s/go-atlas/domain/converter/internal/reflect"
)

func BenchmarkIndirectType_NonPointer(b *testing.B) {
	t := reflect.TypeFor[testStruct]()
	for b.Loop() {
		_ = reflectutils.IndirectType(t)
	}
}

func BenchmarkIndirectType_Pointer(b *testing.B) {
	t := reflect.TypeFor[*testStruct]()
	for b.Loop() {
		_ = reflectutils.IndirectType(t)
	}
}

func BenchmarkIndirectType_DoublePointer(b *testing.B) {
	t := reflect.TypeFor[**testStruct]()
	for b.Loop() {
		_ = reflectutils.IndirectType(t)
	}
}

func BenchmarkIsPrimitive(b *testing.B) {
	for b.Loop() {
		_ = reflectutils.IsPrimitive(reflect.Int)
	}
}
