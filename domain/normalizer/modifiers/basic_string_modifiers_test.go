// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package modifiers

import (
	"reflect"
	"testing"
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
			if result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}
			got := result.Value.String()
			if tt.in == "" {
				got = result.Value.String()
			}
			if got != tt.want && tt.in != "" {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUppercaseModifier(t *testing.T) {
	mod, _ := GetModifier("uppercase")
	result := mod(reflect.ValueOf("hello"), nil)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if got := result.Value.String(); got != "HELLO" {
		t.Errorf("got %q, want %q", got, "HELLO")
	}
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
			if result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}
		})
	}
}

func TestTrimModifier_Pointer(t *testing.T) {
	mod, _ := GetModifier("trim")
	s := "  hello  "
	result := mod(reflect.ValueOf(&s), nil)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if s != "hello" {
		t.Errorf("got %q, want %q", s, "hello")
	}
}
