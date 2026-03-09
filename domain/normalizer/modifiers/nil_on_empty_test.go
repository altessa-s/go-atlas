// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package modifiers

import (
	"reflect"
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
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
		{"*string empty", testhelpers.StringPtr(""), true},
		{"*string non-empty", testhelpers.StringPtr("hello"), false},
		{"int unchanged", 42, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NilOnEmpty(reflect.ValueOf(tt.in), nil)
			if result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}
			if tt.wantNil && result.Value.Kind() == reflect.Pointer && !result.Value.IsNil() {
				t.Error("expected nil pointer")
			}
		})
	}
}
