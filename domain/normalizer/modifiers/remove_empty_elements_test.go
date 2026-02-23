// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package modifiers_test

import (
	"reflect"
	"testing"

	"github.com/altessa-s/go-atlas/domain/normalizer/modifiers"
)

func TestRemoveEmptyElements_StringSlice(t *testing.T) {
	input := []string{"a", "", "b", "  ", "c"}
	v := reflect.ValueOf(&input).Elem()
	result := modifiers.RemoveEmptyElements(v, nil)
	if result.Error != nil {
		t.Fatalf("RemoveEmptyElements() error = %v", result.Error)
	}
	got := result.Value.Interface().([]string)
	if len(got) != 3 {
		t.Errorf("RemoveEmptyElements() len = %d, want 3, got %v", len(got), got)
	}
}

func TestRemoveEmptyElements_PtrStringSlice(t *testing.T) {
	a := "a"
	b := ""
	c := "c"
	input := []*string{&a, &b, &c, nil}
	v := reflect.ValueOf(&input).Elem()
	result := modifiers.RemoveEmptyElements(v, nil)
	if result.Error != nil {
		t.Fatalf("RemoveEmptyElements() error = %v", result.Error)
	}
}

func TestRemoveEmptyElementsFromSlice_StringSlice(t *testing.T) {
	input := []string{"a", "", "b"}
	v := reflect.ValueOf(&input).Elem()
	changed := modifiers.RemoveEmptyElementsFromSlice(v)
	if !changed {
		t.Error("RemoveEmptyElementsFromSlice should return true when elements removed")
	}
	got := v.Interface().([]string)
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestRemoveEmptyElementsFromSlice_NoChange(t *testing.T) {
	input := []string{"a", "b"}
	v := reflect.ValueOf(&input).Elem()
	changed := modifiers.RemoveEmptyElementsFromSlice(v)
	if changed {
		t.Error("RemoveEmptyElementsFromSlice should return false when no change")
	}
}

func TestModifierError_Error(t *testing.T) {
	err := &modifiers.ModifierError{
		FieldName:    "Name",
		ModifierName: "trim",
		Cause:        nil,
	}
	got := err.Error()
	if got == "" {
		t.Error("Error() should not be empty")
	}
}

func TestModifierError_Unwrap(t *testing.T) {
	inner := &modifiers.ModifierError{FieldName: "inner"}
	err := &modifiers.ModifierError{
		FieldName: "outer",
		Cause:     inner,
	}
	if err.Unwrap() != inner {
		t.Error("Unwrap() should return Cause")
	}
}
