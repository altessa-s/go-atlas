// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package maps_test

import (
	"fmt"
	"testing"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

func BenchmarkMerge(b *testing.B) {
	sizes := []int{10, 100, 1000}
	for _, size := range sizes {
		src := make(map[string]int, size)
		dst := make(map[string]int, size)
		for i := range size {
			src[fmt.Sprintf("k%d", i)] = i
			dst[fmt.Sprintf("k%d", i+size)] = i // Disjoint keys
		}

		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			for b.Loop() {
				coremaps.Merge(src, dst)
			}
		})
	}
}

func BenchmarkFilterMap(b *testing.B) {
	sizes := []int{100, 1000}
	for _, size := range sizes {
		input := make(map[string]int, size)
		for i := range size {
			input[fmt.Sprintf("k%d", i)] = i
		}

		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			for b.Loop() {
				coremaps.FilterMap(input, func(k string, v int) bool {
					return v%2 == 0
				})
			}
		})
	}
}

func BenchmarkFlatMap(b *testing.B) {
	nested := map[string]any{
		"a": map[string]any{
			"b": 1,
			"c": map[string]any{
				"d": 2,
				"e": map[string]any{"f": 3},
			},
		},
		"g": 4,
	}

	b.Run("ToFlatMap", func(b *testing.B) {
		for b.Loop() {
			coremaps.ToFlatMap(nested, nil)
		}
	})

	flat := map[string]any{
		"a.b":     1,
		"a.c.d":   2,
		"a.c.e.f": 3,
		"g":       4,
	}

	b.Run("FromFlatMap", func(b *testing.B) {
		for b.Loop() {
			coremaps.FromFlatMap(flat)
		}
	})
}
