// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package reflect_test

import (
	"reflect"
	"testing"

	reflectutils "github.com/altessa-s/go-atlas/domain/converter/internal/reflect"
)

func FuzzIsPrimitive(f *testing.F) {
	// Seed with all valid Kind values (0-26)
	for i := range 27 {
		f.Add(uint8(i))
	}

	f.Fuzz(func(t *testing.T, k uint8) {
		kind := reflect.Kind(k)
		// Should not panic for any kind value
		_ = reflectutils.IsPrimitive(kind)
	})
}
