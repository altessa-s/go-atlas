// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package modifiers

import (
	"reflect"
	"testing"
)

func BenchmarkLowercaseModifier(b *testing.B) {
	mod, _ := GetModifier("lowercase")
	v := reflect.ValueOf("Hello World")
	for b.Loop() {
		mod(v, nil)
	}
}

func BenchmarkNormalizePhone(b *testing.B) {
	v := reflect.ValueOf("+79161234567")
	for b.Loop() {
		NormalizePhone(v, nil)
	}
}
