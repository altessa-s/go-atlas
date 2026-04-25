// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slices_test

import (
	"fmt"
	"testing"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

func BenchmarkDeduplicate(b *testing.B) {
	sizes := []int{10, 100, 1000, 10000}
	for _, size := range sizes {
		input := make([]int, size)
		for i := range size {
			input[i] = i % (size / 2) // Guarantee 50% duplicates
		}

		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			for b.Loop() {
				coreslices.Deduplicate(input)
			}
		})
	}
}

func BenchmarkFilterParallel(b *testing.B) {
	sizes := []int{100, 1000, 10000, 100000}
	for _, size := range sizes {
		input := make([]int, size)
		for i := range size {
			input[i] = i
		}
		predicate := func(n int) bool { return n%2 == 0 }

		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			for b.Loop() {
				coreslices.FilterParallel(input, predicate)
			}
		})
	}
}

func BenchmarkMapParallel(b *testing.B) {
	sizes := []int{100, 1000, 10000, 100000}
	for _, size := range sizes {
		input := make([]int, size)
		for i := range size {
			input[i] = i
		}
		fn := func(n int) int { return n * 2 }

		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			for b.Loop() {
				coreslices.MapParallel(input, fn)
			}
		})
	}
}
