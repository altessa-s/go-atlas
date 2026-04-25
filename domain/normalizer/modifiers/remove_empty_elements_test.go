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
