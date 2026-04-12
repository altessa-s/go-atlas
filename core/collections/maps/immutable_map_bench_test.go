// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package maps_test

import (
	"fmt"
	"runtime"
	"testing"
	"unsafe"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

var benchSizes = []int{100, 1_000, 10_000, 100_000, 1_000_000}

func buildStringMap(n int) (map[string]int, []string) {
	src := make(map[string]int, n)
	keys := make([]string, 0, n)
	for i := range n {
		k := fmt.Sprintf("key-%d", i)
		src[k] = i
		keys = append(keys, k)
	}
	return src, keys
}

// --- Memory measurement ---

func measureHeap(setup func()) uint64 {
	runtime.GC()
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	setup()

	runtime.GC()
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	if after.HeapAlloc > before.HeapAlloc {
		return after.HeapAlloc - before.HeapAlloc
	}
	return 0
}

func TestImmutableMap_MemoryPerKey(t *testing.T) {
	// Serial — heap measurement is incompatible with t.Parallel.
	sizes := []int{10_000, 100_000, 1_000_000}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("Size_%d", size), func(t *testing.T) {
			// Measure ImmutableMap via TotalAlloc (monotonically increasing).
			src := make(map[int]int, size)
			for i := range size {
				src[i] = i
			}

			runtime.GC()
			var before runtime.MemStats
			runtime.ReadMemStats(&before)

			im := coremaps.NewImmutableMap(src)
			runtime.KeepAlive(im)

			var after runtime.MemStats
			runtime.ReadMemStats(&after)
			imBytes := after.TotalAlloc - before.TotalAlloc

			// Measure stdlib map.
			runtime.GC()
			runtime.ReadMemStats(&before)

			stdm := make(map[int]int, size)
			for i := range size {
				stdm[i] = i
			}
			runtime.KeepAlive(stdm)

			runtime.ReadMemStats(&after)
			stdBytes := after.TotalAlloc - before.TotalAlloc

			imPerKey := float64(imBytes) / float64(size)
			stdPerKey := float64(stdBytes) / float64(size)
			ratio := stdPerKey / imPerKey

			t.Logf("n=%d  ImmutableMap=%.1f B/key  stdlib map=%.1f B/key  savings=%.2fx  (key=int %d B, val=int %d B)",
				size, imPerKey, stdPerKey, ratio, unsafe.Sizeof(int(0)), unsafe.Sizeof(int(0)))
		})
	}
}

// --- Get benchmarks ---

func BenchmarkImmutableMap_Get(b *testing.B) {
	for _, size := range benchSizes {
		src, keys := buildStringMap(size)
		m := coremaps.NewImmutableMap(src)

		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				m.Get(keys[i%size])
				i++
			}
		})
	}
}

func BenchmarkStdMap_Get(b *testing.B) {
	for _, size := range benchSizes {
		src, keys := buildStringMap(size)

		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				_ = src[keys[i%size]]
				i++
			}
		})
	}
}

func BenchmarkImmutableMap_Get_Miss(b *testing.B) {
	for _, size := range benchSizes {
		src, _ := buildStringMap(size)
		m := coremaps.NewImmutableMap(src)

		missKeys := make([]string, size)
		for i := range size {
			missKeys[i] = fmt.Sprintf("miss-%d", i)
		}

		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				m.Get(missKeys[i%size])
				i++
			}
		})
	}
}

func BenchmarkStdMap_Get_Miss(b *testing.B) {
	for _, size := range benchSizes {
		src, _ := buildStringMap(size)

		missKeys := make([]string, size)
		for i := range size {
			missKeys[i] = fmt.Sprintf("miss-%d", i)
		}

		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				_ = src[missKeys[i%size]]
				i++
			}
		})
	}
}

// --- Build benchmarks ---

func BenchmarkImmutableMap_Build(b *testing.B) {
	for _, size := range benchSizes {
		src, _ := buildStringMap(size)

		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				coremaps.NewImmutableMap(src)
			}
		})
	}
}

// --- Iteration benchmarks ---

func BenchmarkImmutableMap_All(b *testing.B) {
	for _, size := range benchSizes {
		src, _ := buildStringMap(size)
		m := coremaps.NewImmutableMap(src)

		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				for range m.All() {
				}
			}
		})
	}
}

// --- GC pressure benchmarks ---

func BenchmarkGCPressure_ImmutableMap(b *testing.B) {
	const numMaps = 10
	const mapSize = 100_000
	maps := make([]*coremaps.ImmutableMap[int, int], numMaps)
	for i := range maps {
		src := make(map[int]int, mapSize)
		for j := range mapSize {
			src[j+i*mapSize] = j
		}
		maps[i] = coremaps.NewImmutableMap(src)
	}
	b.ResetTimer()
	for b.Loop() {
		runtime.GC()
	}
	runtime.KeepAlive(maps)
}

func BenchmarkGCPressure_StdMap(b *testing.B) {
	const numMaps = 10
	const mapSize = 100_000
	maps := make([]map[int]int, numMaps)
	for i := range maps {
		maps[i] = make(map[int]int, mapSize)
		for j := range mapSize {
			maps[i][j+i*mapSize] = j
		}
	}
	b.ResetTimer()
	for b.Loop() {
		runtime.GC()
	}
	runtime.KeepAlive(maps)
}

// --- Int keys benchmarks ---

func BenchmarkImmutableMap_Get_IntKeys(b *testing.B) {
	for _, size := range benchSizes {
		src := make(map[int]int, size)
		keys := make([]int, 0, size)
		for i := range size {
			src[i] = i
			keys = append(keys, i)
		}
		m := coremaps.NewImmutableMap(src)

		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				m.Get(keys[i%size])
				i++
			}
		})
	}
}

func BenchmarkStdMap_Get_IntKeys(b *testing.B) {
	for _, size := range benchSizes {
		src := make(map[int]int, size)
		keys := make([]int, 0, size)
		for i := range size {
			src[i] = i
			keys = append(keys, i)
		}

		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				_ = src[keys[i%size]]
				i++
			}
		})
	}
}
