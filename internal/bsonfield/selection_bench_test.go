// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsonfield_test

import (
	"reflect"
	"testing"

	"github.com/altessa-s/go-atlas/internal/bsonfield"
)

func BenchmarkInspect(b *testing.B) {
	typ := reflect.TypeFor[node]()
	for b.Loop() {
		bsonfield.Inspect(typ, tagged)
	}
}
