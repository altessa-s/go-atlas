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

func BenchmarkRemoveEmptyElementsFromSlice(b *testing.B) {
	s1, s2, s3, empty := "alpha", "beta", "gamma", ""
	cases := []struct {
		name     string
		template any
	}{
		{"strings/all-populated", []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta"}},
		{"strings/some-empty", []string{"alpha", "", "beta", "  ", "gamma", "delta", "", "theta"}},
		{"ptrs/all-populated", []*string{&s1, &s2, &s3, &s1, &s2, &s3, &s1, &s2}},
		{"ptrs/some-empty", []*string{&s1, nil, &s2, &empty, &s3, &s1, nil, &s2}},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			tmpl := reflect.ValueOf(tc.template)
			v := reflect.New(tmpl.Type()).Elem()
			buf := reflect.MakeSlice(tmpl.Type(), tmpl.Len(), tmpl.Len())
			b.ReportAllocs()
			for b.Loop() {
				reflect.Copy(buf, tmpl)
				v.Set(buf)
				RemoveEmptyElementsFromSlice(v)
			}
		})
	}
}

func BenchmarkNormalizePhone(b *testing.B) {
	v := reflect.ValueOf("+79161234567")
	for b.Loop() {
		NormalizePhone(v, nil)
	}
}
