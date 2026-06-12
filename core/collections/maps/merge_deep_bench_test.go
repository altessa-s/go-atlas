// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package maps_test

import (
	"testing"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

func BenchmarkMergeDeep(b *testing.B) {
	b.Run("shallow_no_overlap", func(b *testing.B) {
		src := map[string]any{"a": 1, "b": 2}
		dst := map[string]any{"c": 3, "d": 4}
		for b.Loop() {
			coremaps.MergeDeep(src, dst)
		}
	})

	b.Run("shallow_with_overlap", func(b *testing.B) {
		src := map[string]any{"host": "prod.db", "port": 5432}
		dst := map[string]any{"host": "localhost", "port": 5432, "timeout": 30}
		for b.Loop() {
			coremaps.MergeDeep(src, dst)
		}
	})

	b.Run("nested_3_levels", func(b *testing.B) {
		src := map[string]any{
			"db": map[string]any{
				"primary": map[string]any{"host": "prod.db"},
			},
		}
		dst := map[string]any{
			"db": map[string]any{
				"primary": map[string]any{"host": "localhost", "port": 5432},
				"replica": map[string]any{"host": "replica.db"},
			},
			"cache": map[string]any{"host": "redis.local"},
		}
		for b.Loop() {
			coremaps.MergeDeep(src, dst)
		}
	})

	b.Run("wide_flat_100_keys", func(b *testing.B) {
		src := make(map[string]any, 50)
		dst := make(map[string]any, 100)
		for i := range 50 {
			src[string(rune('a'+i%26))+string(rune('0'+i/26))] = i
		}
		for i := range 100 {
			dst[string(rune('a'+i%26))+string(rune('0'+i/26))] = i * 10
		}
		for b.Loop() {
			coremaps.MergeDeep(src, dst)
		}
	})
}
