// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"reflect"
	"testing"
	"time"
)

func BenchmarkHasExportedField(b *testing.B) {
	t := reflect.TypeFor[time.Time]()
	for b.Loop() {
		hasExportedField(t)
	}
}
