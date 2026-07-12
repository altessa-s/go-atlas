// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mapslice_test

import (
	"reflect"
	"testing"

	"github.com/altessa-s/go-atlas/domain/converter/codec/mapslice"
)

func BenchmarkValues(b *testing.B) {
	src := reflect.ValueOf(map[string]int{"a": 1, "b": 2, "c": 3})
	dst := reflect.New(reflect.SliceOf(reflect.TypeFor[int]())).Elem()
	nop := func(_ string, _, _ reflect.Value) {}

	b.ReportAllocs()
	for b.Loop() {
		dst.SetLen(0)
		mapslice.Values("field", src, dst, nop)
	}
}

func BenchmarkKeys(b *testing.B) {
	src := reflect.ValueOf(map[string]int{"a": 1, "b": 2, "c": 3})
	dst := reflect.New(reflect.SliceOf(reflect.TypeFor[string]())).Elem()
	nop := func(_ string, _, _ reflect.Value) {}

	b.ReportAllocs()
	for b.Loop() {
		dst.SetLen(0)
		mapslice.Keys("field", src, dst, nop)
	}
}
