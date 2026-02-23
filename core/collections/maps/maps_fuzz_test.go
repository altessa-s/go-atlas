// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package maps_test

import (
	"testing"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

func FuzzMerge(f *testing.F) {
	f.Add("k1", 1, "k2", 2)

	f.Fuzz(func(t *testing.T, k1 string, v1 int, k2 string, v2 int) {
		src := map[string]int{k1: v1}
		dst := map[string]int{k2: v2}

		merged := coremaps.Merge(src, dst)

		if len(merged) < 1 {
			t.Errorf("Merge result too small")
		}

		if val, ok := merged[k1]; !ok || val != v1 {
			t.Errorf("Merge src value missing/wrong")
		}
	})
}

func FuzzFromFlatMap(f *testing.F) {
	f.Add("a.b", 1)
	f.Add("a", 2)

	f.Fuzz(func(t *testing.T, path string, val int) {
		input := map[string]any{
			path: val,
		}
		// Should never panic
		coremaps.FromFlatMap(input)
	})
}
