// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldtracker_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/mapping/fieldtracker"
)

func TestTracker_FieldNameCache_DifferentTagsAcrossTypes(t *testing.T) {
	type A struct {
		ID string `json:"id"`
	}
	type B struct {
		ID string `json:"identifier"`
	}

	tr := fieldtracker.NewTracker()

	changedA := tr.GetChangedFields(&A{ID: "1"}, &A{ID: "2"})
	require.Equal(t, []string{"id"}, changedA)

	changedB := tr.GetChangedFields(&B{ID: "1"}, &B{ID: "2"})
	require.Equal(t, []string{"identifier"}, changedB)
}

func TestTracker_EmbeddedPointerStruct_IsFlattened(t *testing.T) {
	type Base struct {
		UpdatedAt int64 `json:"updated_at"`
	}
	type Person struct {
		*Base
		Name string `json:"name"`
	}

	tr := fieldtracker.NewTracker()

	before := &Person{Base: &Base{UpdatedAt: 1}, Name: "a"}
	after := &Person{Base: &Base{UpdatedAt: 2}, Name: "a"}
	changed := tr.GetChangedFields(before, after)

	require.Equal(t, []string{"updated_at"}, changed)
}

func TestTracker_SliceAndMap_PrimitiveDiffs_ReturnIndexedPaths(t *testing.T) {
	type S struct {
		Nums []int            `json:"nums"`
		Tags map[string]bool  `json:"tags"`
		IDs  map[int64]string `json:"ids"`
	}

	tr := fieldtracker.NewTracker()

	before := &S{
		Nums: []int{1, 2, 3},
		Tags: map[string]bool{"a": true, "b": false},
		IDs:  map[int64]string{1: "x"},
	}
	after := &S{
		Nums: []int{1, 9, 3},
		Tags: map[string]bool{"a": false, "b": false},
		IDs:  map[int64]string{}, // removed key 1
	}

	changed := tr.GetChangedFields(before, after)

	// We don't assert order (maps are randomized); just check membership.
	want := map[string]struct{}{
		"nums[1]": {},
		"tags[a]": {},
		"ids[1]":  {},
	}

	require.Len(t, changed, len(want))

	for _, p := range changed {
		_, ok := want[p]
		require.True(t, ok, "changed contains unexpected path %q; full=%v", p, changed)
	}
}

func TestTracker_Options(t *testing.T) {
	type Data struct {
		Name    string `json:"name"`
		Ignored string `json:"ignored"`
		Nested  struct {
			Val int `json:"val"`
		} `json:"nested"`
	}

	t.Run("IgnoreFields", func(t *testing.T) {
		tr := fieldtracker.NewTracker(fieldtracker.WithIgnoreFields("ignored"))
		before := &Data{Name: "A", Ignored: "A", Nested: struct {
			Val int `json:"val"`
		}{Val: 1}}
		after := &Data{Name: "B", Ignored: "B", Nested: struct {
			Val int `json:"val"`
		}{Val: 1}}

		changed := tr.GetChangedFields(before, after)
		require.Equal(t, []string{"name"}, changed)
	})

	t.Run("MaxDepth", func(t *testing.T) {
		// A tracker at MaxDepth(1) descends into l1 but not into l2: the difference below the limit is
		// reported at the enclosing path (l1.l2), not dropped.
		tr := fieldtracker.NewTracker(fieldtracker.WithMaxDepth(1))

		type Deep struct {
			L1 struct {
				L2 struct {
					Val int `json:"val"`
				} `json:"l2"`
			} `json:"l1"`
		}

		before := &Deep{}
		after := &Deep{}
		after.L1.L2.Val = 1

		changed := tr.GetChangedFields(before, after)
		require.Equal(t, []string{"l1.l2"}, changed, "a change beyond the depth limit is reported at the enclosing path")

		// Now test with MaxDepth = 2
		tr2 := fieldtracker.NewTracker(fieldtracker.WithMaxDepth(2))
		changed2 := tr2.GetChangedFields(before, after)
		require.Equal(t, []string{"l1.l2.val"}, changed2)
	})
}

func TestTracker_InterfaceDynamicTypeChange(t *testing.T) {
	t.Parallel()

	type A struct {
		X int `json:"x"`
	}
	type B struct {
		Name string `json:"name"`
		X    int    `json:"x"`
	}
	type S struct {
		V any            `json:"v"`
		M map[string]any `json:"m"`
	}

	tests := []struct {
		name          string
		before, after S
		want          []string
	}{
		{"int to string", S{V: 1}, S{V: "x"}, []string{"v"}},
		{"struct to other struct", S{V: A{X: 1}}, S{V: B{Name: "n", X: 1}}, []string{"v"}},
		{"same type value change", S{V: 1}, S{V: 2}, []string{"v"}},
		{"same type same value", S{V: 1}, S{V: 1}, nil},
		{"map value int to string", S{M: map[string]any{"k": 1}}, S{M: map[string]any{"k": "x"}}, []string{"m[k]"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got []string
			require.NotPanics(t, func() {
				got = fieldtracker.GetChangedFields(&tc.before, &tc.after)
			})
			if tc.want == nil {
				require.Empty(t, got)
				return
			}
			require.Equal(t, tc.want, got)
		})
	}
}

func TestTracker_SliceOfOpaqueStructs(t *testing.T) {
	t.Parallel()

	type Item struct {
		Name string `json:"name"`
	}
	type S struct {
		T     []time.Time `json:"t"`
		Items []Item      `json:"items"`
	}

	tests := []struct {
		name          string
		before, after S
		want          []string
	}{
		{"time element changed", S{T: []time.Time{time.Unix(1, 0)}}, S{T: []time.Time{time.Unix(2, 0)}}, []string{"t[0]"}},
		{"time element unchanged", S{T: []time.Time{time.Unix(1, 0)}}, S{T: []time.Time{time.Unix(1, 0)}}, nil},
		{"trackable element field changed", S{Items: []Item{{Name: "a"}}}, S{Items: []Item{{Name: "b"}}}, []string{"items[0].name"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := fieldtracker.GetChangedFields(&tc.before, &tc.after)
			if tc.want == nil {
				require.Empty(t, got)
				return
			}
			require.Equal(t, tc.want, got)
		})
	}
}

func TestInterfaceMapKeyPaths(t *testing.T) {
	t.Parallel()
	type record struct{ Labels map[any]int }
	before := record{Labels: map[any]int{"region": 1, "team": 1, "1": 1, 1: 1, "@int:1": 1, nil: 1}}
	after := record{Labels: map[any]int{"region": 2, "team": 2, "1": 2, 1: 2, "@int:1": 2, nil: 2}}
	tr := fieldtracker.NewTracker(fieldtracker.WithIgnoreFields("labels[region]"))
	require.ElementsMatch(t, []string{"labels[team]", "labels[1]", "labels[@int:1]", "labels[@@int:1]", "labels[@nil]"}, tr.GetChangedFields(&before, &after))
}

type mapKeyLabel struct{ id int }

func (mapKeyLabel) GoString() string { return "label" }

func TestMapKeyPathsKeepSegmentBoundaries(t *testing.T) {
	t.Parallel()
	type record struct{ Labels map[any]any }
	before := record{map[any]any{"x][y": 1, "x": map[string]int{"y": 1}}}
	after := record{map[any]any{"x][y": 2, "x": map[string]int{"y": 2}}}
	require.ElementsMatch(t, []string{`labels[x\]\[y]`, "labels[x][y]"}, fieldtracker.GetChangedFields(before, after))
	require.Equal(t, []string{`labels[x\]\[y]`}, fieldtracker.GetChangedFields(before, after, fieldtracker.WithIgnoreFields("labels[x][y]")))
}

func TestMapKeyPathsKeepGoIdentity(t *testing.T) {
	t.Parallel()
	type record struct {
		Pointers map[*mapKeyLabel]int
		Values   map[mapKeyLabel]int
	}
	a, b := &mapKeyLabel{1}, &mapKeyLabel{1}
	before := record{map[*mapKeyLabel]int{a: 1, b: 1}, map[mapKeyLabel]int{{1}: 1, {2}: 1}}
	after := record{map[*mapKeyLabel]int{a: 2, b: 2}, map[mapKeyLabel]int{{1}: 2, {2}: 2}}
	changed := fieldtracker.GetChangedFields(before, after)
	require.Len(t, changed, 4)
	unique := make(map[string]struct{})
	for _, path := range changed {
		unique[path] = struct{}{}
	}
	require.Len(t, unique, 4)
}
