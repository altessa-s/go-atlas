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
)

func TestFieldMask_IsEmpty(t *testing.T) {
	empty := fieldmask.FromPaths()
	assert.True(t, empty.IsEmpty(), "FromPaths() should be empty")

	nonEmpty := fieldmask.FromPaths("a")
	assert.False(t, nonEmpty.IsEmpty(), "FromPaths('a') should not be empty")
}

func TestFieldMask_Contains_EmptyPath(t *testing.T) {
	m := fieldmask.FromPaths("a")
	assert.False(t, m.Contains(""), "Contains('') should return false")
}

func TestFieldMask_Contains_EmptyMask(t *testing.T) {
	m := fieldmask.FromPaths()
	assert.False(t, m.Contains("a"), "empty mask Contains('a') should return false")
}

func TestFieldMask_Contains_NestedPath(t *testing.T) {
	m := fieldmask.FromPaths("a.b.c")
	assert.True(t, m.Contains("a.b.c"), "should contain a.b.c")
	assert.True(t, m.Contains("a.b"), "should contain a.b (parent)")
	assert.True(t, m.Contains("a"), "should contain a (grandparent)")
	assert.False(t, m.Contains("a.b.c.d"), "should not contain a.b.c.d (deeper than leaf)")
	assert.False(t, m.Contains("x"), "should not contain x")
}

func TestFieldMask_ToProtoFieldMask(t *testing.T) {
	m := fieldmask.FromPaths("a", "b.c")
	pb := m.ToProtoFieldMask()
	require.NotNil(t, pb)
	paths := pb.GetPaths()
	slices.Sort(paths)
	assert.Equal(t, []string{"a", "b.c"}, paths)
}

func TestFieldMask_ToPaths_Empty(t *testing.T) {
	m := fieldmask.FromPaths()
	assert.Empty(t, m.ToPaths())
}

func TestFieldMask_Clone_Empty(t *testing.T) {
	m := fieldmask.FromPaths()
	c := m.Clone()
	assert.True(t, c.IsEmpty(), "clone of empty mask should be empty")
}

func TestFieldMask_Union_Empty(t *testing.T) {
	m := fieldmask.FromPaths("a")
	empty := fieldmask.FromPaths()

	u1 := m.Union(empty)
	assert.True(t, u1.Contains("a"), "union with empty should preserve fields")

	u2 := empty.Union(m)
	assert.True(t, u2.Contains("a"), "empty union with mask should have fields")
}

func TestFieldMask_Intersection_Empty(t *testing.T) {
	m := fieldmask.FromPaths("a")
	empty := fieldmask.FromPaths()

	i := m.Intersection(empty)
	assert.True(t, i.IsEmpty(), "intersection with empty should be empty")
}

func TestFieldMask_Difference_Empty(t *testing.T) {
	m := fieldmask.FromPaths("a", "b")
	empty := fieldmask.FromPaths()

	d1 := m.Difference(empty)
	assert.True(t, d1.Contains("a"), "difference from empty should preserve a")
	assert.True(t, d1.Contains("b"), "difference from empty should preserve b")

	d2 := empty.Difference(m)
	assert.True(t, d2.IsEmpty(), "empty difference should be empty")
}

func TestFieldMask_Validate_NilMessage(t *testing.T) {
	m := fieldmask.FromPaths("a")
	assert.NoError(t, m.Validate(nil))
}

func TestFieldMask_Validate_EmptyMask(t *testing.T) {
	m := fieldmask.FromPaths()
	assert.NoError(t, m.Validate(nil))
}

func TestFieldMask_Filter_EmptyMask(t *testing.T) {
	m := fieldmask.FromPaths()
	// Should not panic.
	m.Filter(nil)
}

func TestFieldMask_Prune_EmptyMask(t *testing.T) {
	m := fieldmask.FromPaths()
	// Should not panic.
	m.Prune(nil)
}

func TestValidationError(t *testing.T) {
	err := &fieldmask.ValidationError{
		Path:   "a.b",
		Reason: "not found",
	}
	assert.Equal(t, "invalid field mask path 'a.b': not found", err.Error())
}

func TestFromPaths_TrailingDot(t *testing.T) {
	m := fieldmask.FromPaths("a.")
	// "a." should be treated as leaf "a" (rest is empty after dot).
	assert.True(t, m.Contains("a"), "'a.' should result in field 'a'")
}

func TestFromPaths_DeeplyNested(t *testing.T) {
	m := fieldmask.FromPaths("a.b.c.d.e")
	paths := m.ToPaths()
	assert.Equal(t, []string{"a.b.c.d.e"}, paths)
}

func TestPrunePaths_NoPaths(t *testing.T) {
	// With no paths, should return original message.
	got := fieldmask.PrunePaths(nil)
	assert.Nil(t, got)
}
