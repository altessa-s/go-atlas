// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nilcheck_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

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
			require.Equal(t, tt.want, nilcheck.IsNil(tt.input))
			require.Equal(t, !tt.want, nilcheck.IsNotNil(tt.input))
		})
	}
}

func TestIsNilValue(t *testing.T) {
	var nilPtr *int = nil
	rv := reflect.ValueOf(nilPtr)

	require.True(t, nilcheck.IsNilValue(rv), "IsNilValue(nilPtr) should be true")
	require.False(t, nilcheck.IsNilValue(reflect.ValueOf(123)), "IsNilValue(123) should be false")
}

func TestRequireNotNil(t *testing.T) {
	require.Error(t, nilcheck.RequireNotNil(nil, "field"))
	require.NoError(t, nilcheck.RequireNotNil(1, "field"))
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
			require.Equal(t, tt.want, nilcheck.IsEmptyValue(reflect.ValueOf(tt.val)))
		})
	}

	// Check pointer logic which uses isStringEmptyOrWhitespace
	s := "   "
	require.True(t, nilcheck.IsEmptyValue(reflect.ValueOf(&s)), "IsEmptyValue(&whitespace) should be true for pointer to string per doc")
}
