// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package modifiers

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNilOnEmpty(t *testing.T) {
	tests := []struct {
		name    string
		in      any
		wantNil bool
	}{
		{"string non-empty", "hello", false},
		{"string empty", "", false}, // string cannot become nil
		{"*string nil", (*string)(nil), true},
		{"*string empty", new(""), true},
		{"*string non-empty", new("hello"), false},
		{"int unchanged", 42, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NilOnEmpty(reflect.ValueOf(tt.in), nil)
			require.Nil(t, result.Error)
			if tt.wantNil && result.Value.Kind() == reflect.Pointer {
				require.True(t, result.Value.IsNil(), "expected nil pointer")
			}
		})
	}
}
