// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAttributeConstructors(t *testing.T) {
	tests := []struct {
		name string
		attr Attribute
		key  string
		val  any
	}{
		{"String", String("k", "v"), "k", "v"},
		{"Int", Int("k", 42), "k", 42},
		{"Int64", Int64("k", 100), "k", int64(100)},
		{"Float64", Float64("k", 3.14), "k", 3.14},
		{"Bool", Bool("k", true), "k", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.key, tt.attr.Key)
			require.Equal(t, tt.val, tt.attr.Value)
		})
	}
}

func TestAttribute_Valid(t *testing.T) {
	tests := []struct {
		name string
		attr Attribute
		want bool
	}{
		{"valid", String("key", "val"), true},
		{"empty key", Attribute{Key: "", Value: "val"}, false},
		{"zero value", Attribute{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.attr.Valid())
		})
	}
}

func TestSliceAttributes(t *testing.T) {
	strSlice := StringSlice("k", []string{"a", "b"})
	require.Equal(t, "k", strSlice.Key)

	intSlice := IntSlice("k", []int{1, 2})
	require.Equal(t, "k", intSlice.Key)

	int64Slice := Int64Slice("k", []int64{1, 2})
	require.Equal(t, "k", int64Slice.Key)

	float64Slice := Float64Slice("k", []float64{1.0, 2.0})
	require.Equal(t, "k", float64Slice.Key)

	boolSlice := BoolSlice("k", []bool{true, false})
	require.Equal(t, "k", boolSlice.Key)
}

func TestStringSlice_DoesNotMutateOriginal(t *testing.T) {
	orig := []string{"a", "b"}
	attr := StringSlice("k", orig)
	cloned := attr.Value.([]string)
	cloned[0] = "modified"
	require.Equal(t, "a", orig[0], "StringSlice should clone the input")
}

func TestCloneAttributes(t *testing.T) {
	orig := []Attribute{String("a", "1"), Int("b", 2)}
	clone := CloneAttributes(orig)
	clone[0] = String("modified", "val")
	require.Equal(t, "a", orig[0].Key, "CloneAttributes should create independent copy")
}

func TestAttributesPool(t *testing.T) {
	attrs := GetAttributes()
	require.NotNil(t, attrs, "GetAttributes returned nil")
	*attrs = append(*attrs, String("k", "v"))
	PutAttributes(attrs)

	attrs2 := GetAttributesWithCapacity(32)
	require.NotNil(t, attrs2, "GetAttributesWithCapacity returned nil")
	PutAttributes(attrs2)
}

func TestAttributeKeys_Iterator(t *testing.T) {
	attrs := []Attribute{String("a", "1"), Int("b", 2), Bool("c", true)}
	keys := slices.Collect(AttributeKeys(attrs))
	want := []string{"a", "b", "c"}
	require.True(t, slices.Equal(keys, want), "keys = %v, want %v", keys, want)
}

func TestFilterAttributesByKey(t *testing.T) {
	attrs := []Attribute{
		String("http.method", "GET"),
		String("http.path", "/api"),
		String("db.system", "postgres"),
	}
	var filtered []Attribute
	for a := range FilterAttributesByKey(attrs, "http.") {
		filtered = append(filtered, a)
	}
	require.Len(t, filtered, 2, "expected 2 http attrs")
}
