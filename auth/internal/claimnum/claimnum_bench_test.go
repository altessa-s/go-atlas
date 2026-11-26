// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package claimnum_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/auth/internal/claimnum"
)

func BenchmarkDecode(b *testing.B) {
	data := []byte(`{"tenant":9007199254740993,"level":3,"ratio":0.25,"groups":["a","b"],"nested":{"n":42}}`)
	for b.Loop() {
		var m map[string]any
		if err := claimnum.Decode(data, &m); err != nil {
			b.Fatal(err)
		}
	}
}
