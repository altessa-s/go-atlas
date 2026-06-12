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

func BenchmarkMergeWith(b *testing.B) {
	sizes := []int{10, 100, 1000}
	for _, size := range sizes {
		src := make(map[string]int, size)
		dst := make(map[string]int, size)
		for i := range size {
			src[fmt.Sprintf("k%d", i)] = i
			dst[fmt.Sprintf("k%d", i)] = i * 2 // overlapping keys
		}
		resolve := func(_ string, s, d int) int { return s + d }

		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			for b.Loop() {
				coremaps.MergeWith(src, dst, resolve)
			}
		})
	}
}

func BenchmarkMergeAll(b *testing.B) {
	const layerSize = 100
	for _, numLayers := range []int{2, 5, 10} {
		layers := make([]map[string]int, numLayers)
		for i := range numLayers {
			m := make(map[string]int, layerSize)
			for j := range layerSize {
				m[fmt.Sprintf("k%d", j)] = i*layerSize + j
			}
			layers[i] = m
		}

		b.Run(fmt.Sprintf("Layers_%d", numLayers), func(b *testing.B) {
			for b.Loop() {
				coremaps.MergeAll(layers...)
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
