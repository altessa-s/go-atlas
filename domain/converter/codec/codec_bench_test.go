// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package convcodec

import (
	"reflect"
	"testing"
)

func BenchmarkSet_Run_Chain(b *testing.B) {
	c1 := Codec(func(_ string, src, dst reflect.Value, next CodecHandler) { next("", src, dst) })
	c2 := Codec(func(_ string, src, dst reflect.Value, next CodecHandler) { next("", src, dst) })
	c3 := Codec(func(_ string, src, dst reflect.Value, next CodecHandler) { next("", src, dst) })
	s := NewCodecsSet(c1, c2, c3)

	src := reflect.ValueOf(42)
	dst := reflect.ValueOf(0)
	handler := func(_ string, _, _ reflect.Value) {}

	for b.Loop() {
		s.Run("field", src, dst, handler)
	}
}
