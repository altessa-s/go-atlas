// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nilcheck_test

import (
	"reflect"
	"testing"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"
)

func TestIsNil(t *testing.T) {
	var (
		nilPtr      *int = nil
		nonNilPtr   *int = new(int)
		nilIface    any  = nil
		typedNil    any  = nilPtr
		nonNilIface any  = nonNilPtr
	)

	tests := []struct {
		name  string
		input any
		want  bool
	}{
		{"nil interface", nilIface, true},
		{"typed nil", typedNil, true},
		{"non-nil pointer", nonNilPtr, false},
		{"non-nil interface", nonNilIface, false},
		{"nil slice", ([]int)(nil), true},
		{"nil map", (map[int]int)(nil), true},
		{"empty struct", struct{}{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nilcheck.IsNil(tt.input); got != tt.want {
				t.Errorf("IsNil() = %v, want %v", got, tt.want)
			}
			if got := nilcheck.IsNotNil(tt.input); got == tt.want {
				t.Errorf("IsNotNil() = %v, want %v", got, !tt.want)
			}
		})
	}
}

func TestIsNilValue(t *testing.T) {
	var nilPtr *int = nil
	rv := reflect.ValueOf(nilPtr)

	if !nilcheck.IsNilValue(rv) {
		t.Error("IsNilValue(nilPtr) should be true")
	}

	if nilcheck.IsNilValue(reflect.ValueOf(123)) {
		t.Error("IsNilValue(123) should be false")
	}
}

func TestRequireNotNil(t *testing.T) {
	if err := nilcheck.RequireNotNil(nil, "field"); err == nil {
		t.Error("RequireNotNil(nil) should return error")
	}
	if err := nilcheck.RequireNotNil(1, "field"); err != nil {
		t.Error("RequireNotNil(1) should not return error")
	}
}

func TestIsEmptyValue(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want bool
	}{
		{"zero int", 0, true},
		{"non-zero int", 1, false},
		{"empty string", "", true},
		{"whitespace string", "   ", false}, // IsEmptyValue for string is strict? No, doc says "string itself is empty (including whitespace)" logic for POINTERS
	}

	// Check strict values first
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nilcheck.IsEmptyValue(reflect.ValueOf(tt.val)); got != tt.want {
				t.Errorf("IsEmptyValue(%v) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}

	// Check pointer logic which uses isStringEmptyOrWhitespace
	s := "   "
	if !nilcheck.IsEmptyValue(reflect.ValueOf(&s)) {
		t.Error("IsEmptyValue(&whitespace) should be true for pointer to string per doc")
	}
}
