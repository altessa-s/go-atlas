// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func TestFromPaths(t *testing.T) {
	tests := []struct {
		name      string
		paths     []string
		wantPaths []string
	}{
		{
			name:      "single path",
			paths:     []string{"user.name"},
			wantPaths: []string{"user.name"},
		},
		{
			name:      "nested paths",
			paths:     []string{"user.name", "user.age"},
			wantPaths: []string{"user.age", "user.name"},
		},
		{
			name:      "leaf dominates nested",
			paths:     []string{"user", "user.details"},
			wantPaths: []string{"user"},
		},
		{
			name:      "empty paths filtered",
			paths:     []string{"a", "", "b"},
			wantPaths: []string{"a", "b"},
		},
		{
			name:      "duplicates",
			paths:     []string{"a.b", "a.b"},
			wantPaths: []string{"a.b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mask := fieldmask.FromPaths(tt.paths...)
			got := mask.ToPaths()
			slices.Sort(got)
			slices.Sort(tt.wantPaths)
			assert.Equal(t, tt.wantPaths, got)
		})
	}
}

func TestFromProtoFieldMask(t *testing.T) {
	pb := &fieldmaskpb.FieldMask{Paths: []string{"a.b", "c"}}
	mask := fieldmask.FromProtoFieldMask(pb)
	got := mask.ToPaths()
	slices.Sort(got)
	assert.Equal(t, []string{"a.b", "c"}, got)
}

func TestSetOperations(t *testing.T) {
	m1 := fieldmask.FromPaths("a", "b.c")
	m2 := fieldmask.FromPaths("b.d", "e")
	m3 := fieldmask.FromPaths("a.x") // Nested under 'a'

	t.Run("Union", func(t *testing.T) {
		// m1 U m2 = {a, b.c, b.d, e}
		u := m1.Union(m2)
		got := u.ToPaths()
		slices.Sort(got)
		assert.Equal(t, []string{"a", "b.c", "b.d", "e"}, got)

		// m1 U m3 = {a, b.c}. 'a' dominates 'a.x', so 'a' is kept. 'b.c' is preserved.
		u2 := m1.Union(m3)
		got2 := u2.ToPaths()
		slices.Sort(got2)
		assert.Equal(t, []string{"a", "b.c"}, got2)
	})

	t.Run("Intersection", func(t *testing.T) {
		ma := fieldmask.FromPaths("a.b", "c")
		mb := fieldmask.FromPaths("a", "c.d")

		// ma ^ mb
		// a.b vs a -> a.b (since a includes a.b)
		// c vs c.d -> c.d (since c includes c.d)
		// Result: {a.b, c.d}
		i := ma.Intersection(mb)
		got := i.ToPaths()
		slices.Sort(got)
		assert.Equal(t, []string{"a.b", "c.d"}, got)

		// Disjoint
		mc := fieldmask.FromPaths("x")
		i2 := ma.Intersection(mc)
		assert.True(t, i2.IsEmpty(), "intersection of disjoint masks should be empty")
	})

	t.Run("Difference", func(t *testing.T) {
		ma := fieldmask.FromPaths("a", "b", "c.d")
		mb := fieldmask.FromPaths("b", "c")

		// ma - mb
		// a - (not present) -> a
		// b - b -> (empty)
		// c.d - c -> (empty, c dominates c.d)
		d := ma.Difference(mb)
		got := d.ToPaths()
		assert.Equal(t, []string{"a"}, got)

		// Simple case
		mx := fieldmask.FromPaths("x", "y")
		my := fieldmask.FromPaths("y")
		dx := mx.Difference(my)
		assert.True(t, dx.Contains("x"))
		assert.False(t, dx.Contains("y"))
	})
}

func TestContains(t *testing.T) {
	m := fieldmask.FromPaths("a.b", "c")

	assert.True(t, m.Contains("a.b"), "should contain a.b")
	assert.True(t, m.Contains("c"), "should contain c")
	assert.True(t, m.Contains("a"), "should contain a (as partial path)")
}

func TestClone(t *testing.T) {
	m := fieldmask.FromPaths("a.b")
	c := m.Clone()

	m2 := m.Union(fieldmask.FromPaths("x"))
	assert.False(t, c.Contains("x"), "clone should be independent")
	require.True(t, m2.Contains("x"), "original should be modifiable")
}
