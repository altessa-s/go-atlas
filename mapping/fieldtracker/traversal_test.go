// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldtracker_test

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/mapping/fieldtracker"

	generationmemory "github.com/altessa-s/go-atlas/cache/generation/storages/memory"
	sagamemory "github.com/altessa-s/go-atlas/reliability/saga/storages/memory"
)

func TestTrackerBoundsCollectionDepth(t *testing.T) {
	t.Parallel()

	type record struct{ Items any }
	cases := []struct {
		name       string
		makeRecord func(int) record
		path       string
	}{
		{
			name: "nested maps",
			makeRecord: func(n int) record {
				return record{map[string]any{"items": map[string]int{"value": n}}}
			},
			path: "items[items][value]",
		},
		{
			name: "nested slices",
			makeRecord: func(n int) record {
				return record{[]any{[]int{n}}}
			},
			path: "items[0][0]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			before, after := tc.makeRecord(1), tc.makeRecord(2)
			require.Empty(t, fieldtracker.GetChangedFields(before, after, fieldtracker.WithMaxDepth(1)))
			require.Equal(t, []string{tc.path}, fieldtracker.GetChangedFields(before, after, fieldtracker.WithMaxDepth(2)))
		})
	}
}

func TestTrackerStopsCyclesWithoutDepthLimit(t *testing.T) {
	t.Parallel()

	type node struct {
		Name  string
		Next  *node
		Map   map[string]any
		Slice []any
	}
	makeNode := func(name string) *node {
		v := &node{Name: name, Map: make(map[string]any), Slice: make([]any, 2)}
		v.Next = v
		v.Map["again"] = v.Map
		v.Map["value"] = name
		v.Slice[0], v.Slice[1] = v.Slice, name
		return v
	}

	before, after := makeNode("old"), makeNode("new")
	tr := fieldtracker.NewTracker(fieldtracker.WithMaxDepth(0))
	require.Empty(t, tr.GetChangedFields(before, makeNode("old")))
	require.ElementsMatch(t, []string{"name", "map[value]", "slice[1]"}, tr.GetChangedFields(before, after))

	// Two independent field paths can point to the same pair. Both paths
	// still report the changes reachable through them.
	type record struct{ Left, Right *node }
	want := []string{"left.name", "left.map[value]", "left.slice[1]", "right.name", "right.map[value]", "right.slice[1]"}
	require.ElementsMatch(t, want, tr.GetChangedFields(record{before, before}, record{after, after}))
}

func TestTrackerStopsDeepCyclesAlongEachPath(t *testing.T) {
	t.Parallel()

	type record struct{ Left, Right map[string]any }
	const depth = 16
	makeRecord := func(value int) record {
		items := map[string]any{"value": value}
		items["again"] = items
		for range depth {
			items = map[string]any{"next": items}
		}
		return record{items, items}
	}

	path := strings.Repeat("[next]", depth) + "[value]"
	tr := fieldtracker.NewTracker(fieldtracker.WithMaxDepth(0))
	require.ElementsMatch(t, []string{"left" + path, "right" + path}, tr.GetChangedFields(makeRecord(1), makeRecord(2)))
}

func TestTrackerEscapesLiteralTagSegments(t *testing.T) {
	t.Parallel()

	type metadata struct {
		Name string `json:"name"`
	}
	type record struct {
		Literal string   `json:"meta.name"`
		Meta    metadata `json:"meta"`
		Indexed string   `json:"items[0]"`
		Items   [1]int   `json:"items"`
	}
	before := record{}
	after := record{Literal: "a", Meta: metadata{"b"}, Indexed: "c", Items: [1]int{1}}
	require.ElementsMatch(t, []string{`meta\.name`, "meta.name", `items\[0\]`, "items[0]"}, fieldtracker.GetChangedFields(before, after))
	require.ElementsMatch(t, []string{`meta\.name`, `items\[0\]`}, fieldtracker.GetChangedFields(before, after,
		fieldtracker.WithIgnoreFields("meta.name", "items[0]")))
}

func TestTrackerKeepsPackageIdentityOfUnnamedKeyTypes(t *testing.T) {
	t.Parallel()

	// The nested named types have the same local name and package name.
	first := struct{ Store *generationmemory.Store }{}
	second := struct{ Store *sagamemory.Store }{}
	type record struct{ Labels map[any]int }
	before := record{map[any]int{first: 1, second: 1}}
	after := record{map[any]int{first: 2, second: 2}}
	paths := fieldtracker.GetChangedFields(before, after)
	require.Len(t, paths, 2)
	require.NotEqual(t, paths[0], paths[1])
	require.Equal(t, paths[1:], fieldtracker.GetChangedFields(before, after, fieldtracker.WithIgnoreFields(paths[0])))
}

func TestScalarPointerAtDepthLimit(t *testing.T) {
	t.Parallel()
	type leaf struct {
		Value int
		Ref   *int
	}
	type record struct{ Leaf leaf }
	paths := fieldtracker.GetChangedFields(record{leaf{1, new(1)}}, record{leaf{2, new(2)}}, fieldtracker.WithMaxDepth(1))
	require.ElementsMatch(t, []string{"leaf.value", "leaf.ref"}, paths)
}

func TestLocalTypesWithSameName(t *testing.T) {
	t.Parallel()
	type record struct{ M map[any]int }
	var left, right any
	{
		type key int
		left = key(1)
	}
	{
		type key int
		right = key(1)
	}
	before := record{map[any]int{left: 0, right: 0}}
	after := record{map[any]int{left: 1, right: 1}}
	paths := fieldtracker.GetChangedFields(before, after)
	require.Len(t, paths, 2)
	require.NotEqual(t, paths[0], paths[1])
	require.Equal(t, paths[1:], fieldtracker.GetChangedFields(after, before, fieldtracker.WithIgnoreFields(paths[0])))
}

func TestSignedZeroKeyPaths(t *testing.T) {
	t.Parallel()
	type record struct{ M map[any]int }
	negative := math.Copysign(0, -1)
	before := record{map[any]int{negative: 0, complex(negative, negative): 0}}
	after := record{map[any]int{0.0: 1, complex(0.0, 0.0): 1}}
	paths := fieldtracker.GetChangedFields(before, after)
	require.Len(t, paths, 2)
	require.ElementsMatch(t, paths, fieldtracker.GetChangedFields(after, before))
}
