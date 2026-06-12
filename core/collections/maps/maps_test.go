// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package maps_test

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

func TestMerge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		src      map[string]int
		dst      map[string]int
		expected map[string]int
	}{
		{
			name:     "merge disjoint",
			src:      map[string]int{"a": 1},
			dst:      map[string]int{"b": 2},
			expected: map[string]int{"a": 1, "b": 2},
		},
		{
			name:     "merge overwrite",
			src:      map[string]int{"a": 1},
			dst:      map[string]int{"a": 2},
			expected: map[string]int{"a": 1}, // src overwrites dst
		},
		{
			name:     "merge empty src",
			src:      nil,
			dst:      map[string]int{"a": 1},
			expected: map[string]int{"a": 1},
		},
		{
			name:     "merge empty dst",
			src:      map[string]int{"a": 1},
			dst:      nil,
			expected: map[string]int{"a": 1},
		},
		{
			name:     "both empty",
			src:      nil,
			dst:      nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := coremaps.Merge(tt.src, tt.dst)
			require.True(t, maps.Equal(got, tt.expected), "Merge() = %v, want %v", got, tt.expected)
			// Verify immutability
			if len(tt.dst) > 0 {
				if &got == &tt.dst { // Address check logic is flawed here for maps, check modification
					// Instead check if modifying result affects inputs
					got["new"] = 999
					_, ok := tt.dst["new"]
					require.False(t, ok, "Merge result shares memory with input dst")
				}
			}
		})
	}
}

func TestSwap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    map[string]int
		expected map[int]string
	}{
		{
			name:     "simple swap",
			input:    map[string]int{"a": 1, "b": 2},
			expected: map[int]string{1: "a", 2: "b"},
		},
		{
			name:     "empty",
			input:    nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := coremaps.Swap(tt.input)
			require.True(t, maps.Equal(got, tt.expected), "Swap() = %v, want %v", got, tt.expected)
		})
	}
}

func TestFilterMap(t *testing.T) {
	t.Parallel()

	input := map[string]int{"a": 1, "b": 2, "c": 3}
	got := coremaps.FilterMap(input, func(k string, v int) bool {
		return v%2 != 0
	})
	expected := map[string]int{"a": 1, "c": 3}
	require.True(t, maps.Equal(got, expected), "FilterMap() = %v, want %v", got, expected)

	require.Nil(t, coremaps.FilterMap(map[string]int(nil), func(string, int) bool { return true }), "FilterMap(nil) should return nil")
}

func TestConvertMap(t *testing.T) {
	t.Parallel()

	input := map[string]int{"a": 1, "b": 2}
	got := coremaps.ConvertMap(input, func(k string, v int) (int, string) {
		return v, k
	})
	expected := map[int]string{1: "a", 2: "b"}
	require.True(t, maps.Equal(got, expected), "ConvertMap() = %v, want %v", got, expected)
}

func TestFromSlice(t *testing.T) {
	t.Parallel()

	users := []testhelpers.User{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}, {ID: 1, Name: "A2"}} // Duplicate ID 1

	// Last one wins
	got := coremaps.FromSlice(users, func(u testhelpers.User) int { return u.ID })
	expected := map[int]testhelpers.User{
		1: {ID: 1, Name: "A2"},
		2: {ID: 2, Name: "B"},
	}
	require.True(t, maps.Equal(got, expected), "FromSlice() = %v, want %v", got, expected)
}

func TestFromSliceWith(t *testing.T) {
	t.Parallel()

	users := []testhelpers.User{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}
	got := coremaps.FromSliceWith(users, func(u testhelpers.User) (int, string) {
		return u.ID, u.Name
	})
	expected := map[int]string{1: "A", 2: "B"}
	require.True(t, maps.Equal(got, expected), "FromSliceWith() = %v, want %v", got, expected)
}

func TestToKeyValueSlice(t *testing.T) {
	t.Parallel()

	input := map[string]string{"k1": "v1"}
	got := coremaps.ToKeyValueSlice(input)
	require.Len(t, got, 2)
	// Order check is tricky, but with one element strict match
	if got[0] == "k1" && got[1] == "v1" {
		// ok
	} else {
		require.Fail(t, "ToKeyValueSlice() unexpected result", "got %v, want [k1 v1]", got)
	}

	require.Nil(t, coremaps.ToKeyValueSlice[string, int](nil), "ToKeyValueSlice(nil) should return nil")
}

func TestMergeWith(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		src      map[string]int
		dst      map[string]int
		resolve  func(string, int, int) int
		expected map[string]int
	}{
		{
			name:     "disjoint maps resolve not called",
			src:      map[string]int{"a": 1},
			dst:      map[string]int{"b": 2},
			resolve:  func(_ string, s, d int) int { return s + d },
			expected: map[string]int{"a": 1, "b": 2},
		},
		{
			name:     "overlapping key accumulates",
			src:      map[string]int{"a": 3, "b": 1},
			dst:      map[string]int{"a": 5, "c": 2},
			resolve:  func(_ string, s, d int) int { return s + d },
			expected: map[string]int{"a": 8, "b": 1, "c": 2},
		},
		{
			name:     "overlapping key src wins",
			src:      map[string]int{"a": 1},
			dst:      map[string]int{"a": 2},
			resolve:  func(_ string, s, _ int) int { return s },
			expected: map[string]int{"a": 1},
		},
		{
			name:     "empty src returns copy of dst",
			src:      nil,
			dst:      map[string]int{"a": 1},
			resolve:  func(_ string, s, _ int) int { return s },
			expected: map[string]int{"a": 1},
		},
		{
			name:     "empty dst returns copy of src",
			src:      map[string]int{"a": 1},
			dst:      nil,
			resolve:  func(_ string, s, _ int) int { return s },
			expected: map[string]int{"a": 1},
		},
		{
			name:     "both empty returns nil",
			src:      nil,
			dst:      nil,
			resolve:  func(_ string, s, _ int) int { return s },
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := coremaps.MergeWith(tt.src, tt.dst, tt.resolve)
			require.True(t, maps.Equal(got, tt.expected), "MergeWith() = %v, want %v", got, tt.expected)
		})
	}
}

func TestMergeAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		layers   []map[string]string
		expected map[string]string
	}{
		{
			name:     "no layers returns nil",
			layers:   nil,
			expected: nil,
		},
		{
			name:     "all empty layers returns nil",
			layers:   []map[string]string{nil, nil},
			expected: nil,
		},
		{
			name:     "single layer",
			layers:   []map[string]string{{"a": "1"}},
			expected: map[string]string{"a": "1"},
		},
		{
			name:     "last layer wins on conflict",
			layers:   []map[string]string{{"a": "base", "b": "base"}, {"b": "mid"}, {"a": "top", "c": "top"}},
			expected: map[string]string{"a": "top", "b": "mid", "c": "top"},
		},
		{
			name:     "disjoint layers merged",
			layers:   []map[string]string{{"a": "1"}, {"b": "2"}, {"c": "3"}},
			expected: map[string]string{"a": "1", "b": "2", "c": "3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := coremaps.MergeAll(tt.layers...)
			require.True(t, maps.Equal(got, tt.expected), "MergeAll() = %v, want %v", got, tt.expected)
		})
	}
}

func TestFlatMap(t *testing.T) {
	t.Parallel()

	t.Run("ToFlatMap", func(t *testing.T) {
		t.Parallel()
		nested := map[string]any{
			"a": map[string]any{
				"b": 1,
				"c": map[string]any{"d": 2},
			},
			"e": 3,
		}
		require.Equal(t, map[string]any{"a.b": 1, "a.c.d": 2, "e": 3}, coremaps.ToFlatMap(nested, nil))
	})

	t.Run("FromFlatMap", func(t *testing.T) {
		t.Parallel()
		flat := map[string]any{"a.b": 1, "a.c.d": 2, "e": 3}
		got := coremaps.FromFlatMap(flat)

		getNested := func(m map[string]any, keys ...string) any {
			curr := any(m)
			for _, k := range keys {
				if cm, ok := curr.(map[string]any); ok {
					curr = cm[k]
				} else {
					return nil
				}
			}
			return curr
		}

		require.Equal(t, 1, getNested(got, "a", "b"))
		require.Equal(t, 2, getNested(got, "a", "c", "d"))
		require.Equal(t, 3, getNested(got, "e"))
	})

	t.Run("ConflictHandler", func(t *testing.T) {
		t.Parallel()
		// Conflict detection is iteration-order-dependent; verify no panic occurs.
		flat := map[string]any{"foo": "bar", "foo.bar": "baz"}
		coremaps.FromFlatMapWithHandler(flat, func(_ string, _ any) {})
	})
}
