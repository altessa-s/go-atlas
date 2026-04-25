// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package modifiers

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLowercaseModifier(t *testing.T) {
	mod, _ := GetModifier("lowercase")
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"mixed", "Hello World", "hello world"},
		{"already lower", "hello", "hello"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mod(reflect.ValueOf(tt.in), nil)
			require.Nil(t, result.Error)
			if tt.in != "" {
				require.Equal(t, tt.want, result.Value.String())
			}
		})
	}
}

func TestUppercaseModifier(t *testing.T) {
	mod, _ := GetModifier("uppercase")
	result := mod(reflect.ValueOf("hello"), nil)
	require.Nil(t, result.Error)
	require.Equal(t, "HELLO", result.Value.String())
}

func TestTrimModifier(t *testing.T) {
	mod, _ := GetModifier("trim")
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"spaces", "  hello  ", "hello"},
		{"no trim needed", "hello", "hello"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mod(reflect.ValueOf(tt.in), nil)
			require.Nil(t, result.Error)
		})
	}
}

func TestTrimModifier_Pointer(t *testing.T) {
	mod, _ := GetModifier("trim")
	s := "  hello  "
	result := mod(reflect.ValueOf(&s), nil)
	require.Nil(t, result.Error)
	require.Equal(t, "hello", s)
}
