// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package maps_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

func TestMergeDeep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		src      map[string]any
		dst      map[string]any
		expected map[string]any
	}{
		{
			name:     "both nil returns nil",
			src:      nil,
			dst:      nil,
			expected: nil,
		},
		{
			name:     "nil src returns copy of dst",
			src:      nil,
			dst:      map[string]any{"a": 1},
			expected: map[string]any{"a": 1},
		},
		{
			name:     "nil dst returns copy of src",
			src:      map[string]any{"a": 1},
			dst:      nil,
			expected: map[string]any{"a": 1},
		},
		{
			name:     "disjoint keys merged",
			src:      map[string]any{"a": 1},
			dst:      map[string]any{"b": 2},
			expected: map[string]any{"a": 1, "b": 2},
		},
		{
			name:     "scalar conflict src wins",
			src:      map[string]any{"a": "src"},
			dst:      map[string]any{"a": "dst"},
			expected: map[string]any{"a": "src"},
		},
		{
			name: "nested maps merged recursively",
			src: map[string]any{
				"db": map[string]any{"host": "prod.db"},
			},
			dst: map[string]any{
				"db":  map[string]any{"host": "localhost", "port": 5432},
				"log": "info",
			},
			expected: map[string]any{
				"db":  map[string]any{"host": "prod.db", "port": 5432},
				"log": "info",
			},
		},
		{
			name:     "src scalar replaces dst nested map",
			src:      map[string]any{"a": "scalar"},
			dst:      map[string]any{"a": map[string]any{"x": 1}},
			expected: map[string]any{"a": "scalar"},
		},
		{
			name:     "src nested map replaces dst scalar",
			src:      map[string]any{"a": map[string]any{"x": 1}},
			dst:      map[string]any{"a": "scalar"},
			expected: map[string]any{"a": map[string]any{"x": 1}},
		},
		{
			name: "deeply nested merge preserves siblings",
			src: map[string]any{
				"l1": map[string]any{
					"l2": map[string]any{"key": "src"},
				},
			},
			dst: map[string]any{
				"l1": map[string]any{
					"l2":      map[string]any{"key": "dst", "other": 42},
					"sibling": "preserved",
				},
			},
			expected: map[string]any{
				"l1": map[string]any{
					"l2":      map[string]any{"key": "src", "other": 42},
					"sibling": "preserved",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, coremaps.MergeDeep(tt.src, tt.dst))
		})
	}
}

func TestMergeDeep_Immutability(t *testing.T) {
	t.Parallel()

	src := map[string]any{
		"db": map[string]any{"host": "prod"},
	}
	dst := map[string]any{
		"db":  map[string]any{"host": "local", "port": 5432},
		"log": map[string]any{"level": "info"},
	}

	result := coremaps.MergeDeep(src, dst)

	// Mutate the result's nested maps.
	result["db"].(map[string]any)["host"] = "mutated"
	result["db"].(map[string]any)["port"] = 9999
	result["log"].(map[string]any)["level"] = "debug"

	// src and dst must be unaffected.
	require.Equal(t, "prod", src["db"].(map[string]any)["host"], "src mutated")
	require.Equal(t, "local", dst["db"].(map[string]any)["host"], "dst db.host mutated")
	require.Equal(t, 5432, dst["db"].(map[string]any)["port"], "dst db.port mutated")
	require.Equal(t, "info", dst["log"].(map[string]any)["level"], "dst log.level mutated")
}
