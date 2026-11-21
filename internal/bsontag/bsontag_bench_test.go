// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsontag

import "testing"

func BenchmarkParse(b *testing.B) {
	for _, tag := range []string{"", "name", ",omitempty", "name,omitempty,minsize,omitonupdate"} {
		b.Run(tag+"/direct", func(b *testing.B) {
			for b.Loop() {
				parse(tag, "FieldName")
			}
		})
		b.Run(tag+"/cache-hit", func(b *testing.B) {
			Parse(tag, "FieldName")
			for b.Loop() {
				Parse(tag, "FieldName")
			}
		})
	}
}
