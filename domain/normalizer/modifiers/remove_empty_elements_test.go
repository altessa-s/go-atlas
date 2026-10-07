// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package modifiers_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/normalizer/modifiers"
)

func TestRemoveEmptyElements_StringSlice(t *testing.T) {
	input := []string{"a", "", "b", "  ", "c"}
	v := reflect.ValueOf(&input).Elem()
	result := modifiers.RemoveEmptyElements(v, nil)
	require.Nil(t, result.Error)
	got := result.Value.Interface().([]string)
	require.Len(t, got, 3)
}

func TestRemoveEmptyElements_PtrStringSlice(t *testing.T) {
	a := "a"
	b := ""
	c := "c"
	input := []*string{&a, &b, &c, nil}
	v := reflect.ValueOf(&input).Elem()
	result := modifiers.RemoveEmptyElements(v, nil)
	require.Nil(t, result.Error)
}

func TestRemoveEmptyElementsFromSlice_StringSlice(t *testing.T) {
	input := []string{"a", "", "b"}
	v := reflect.ValueOf(&input).Elem()
	changed := modifiers.RemoveEmptyElementsFromSlice(v)
	require.True(t, changed, "RemoveEmptyElementsFromSlice should return true when elements removed")
	got := v.Interface().([]string)
	require.Len(t, got, 2)
}

func TestRemoveEmptyElementsFromSlice_NoChange(t *testing.T) {
	input := []string{"a", "b"}
	v := reflect.ValueOf(&input).Elem()
	changed := modifiers.RemoveEmptyElementsFromSlice(v)
	require.False(t, changed, "RemoveEmptyElementsFromSlice should return false when no change")
}

func TestRemoveEmptyElementsFromSlice_PreservesOrderAndBacking(t *testing.T) {
	t.Parallel()

	a, b, c, empty := "a", "b", "c", " "
	tests := []struct {
		name        string
		input       any
		want        any
		wantChanged bool
	}{
		{"strings untouched", []string{"a", "b", "c"}, []string{"a", "b", "c"}, false},
		{"strings leading empty", []string{"", "a", "b"}, []string{"a", "b"}, true},
		{"strings mixed", []string{"a", "b", "", "c", "  "}, []string{"a", "b", "c"}, true},
		{"strings all empty", []string{"", " "}, []string{}, true},
		{"ptrs untouched", []*string{&a, &b}, []*string{&a, &b}, false},
		{"ptrs mixed", []*string{&a, nil, &b, &empty, &c}, []*string{&a, &b, &c}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := reflect.New(reflect.TypeOf(tc.input)).Elem()
			v.Set(reflect.ValueOf(tc.input))
			before := v.Pointer()

			changed := modifiers.RemoveEmptyElementsFromSlice(v)
			require.Equal(t, tc.wantChanged, changed)
			require.Equal(t, tc.want, v.Interface())
			if !tc.wantChanged {
				require.Equal(t, before, v.Pointer(), "unchanged input must keep its backing array")
			}
		})
	}
}

func TestModifierError_Error(t *testing.T) {
	err := &modifiers.ModifierError{
		FieldName:    "Name",
		ModifierName: "trim",
		Cause:        nil,
	}
	got := err.Error()
	require.NotEmpty(t, got)
}

func TestModifierError_Unwrap(t *testing.T) {
	inner := &modifiers.ModifierError{FieldName: "inner"}
	err := &modifiers.ModifierError{
		FieldName: "outer",
		Cause:     inner,
	}
	require.Equal(t, inner, err.Unwrap())
}
