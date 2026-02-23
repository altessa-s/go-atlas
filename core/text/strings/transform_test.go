// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings_test

import (
	"testing"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

func TestToScreamingSnakeCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"camelCase", "CAMEL_CASE"},
		{"PascalCase", "PASCAL_CASE"},
		{"already_snake", "ALREADY_SNAKE"},
		{"HTMLParser", "HTML_PARSER"},
		{"simple", "SIMPLE"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := corestrings.ToScreamingSnakeCase(tt.input); got != tt.want {
				t.Errorf("ToScreamingSnakeCase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"camelCase", "camel_case"},
		{"PascalCase", "pascal_case"},
		{"already_snake", "already_snake"},
		{"simple", "simple"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := corestrings.ToSnakeCase(tt.input); got != tt.want {
				t.Errorf("ToSnakeCase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestToCamelCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello_world", "helloWorld"},
		{"simple", "simple"},
		{"", ""},
		{"SCREAMING_SNAKE", "sCREAMINGSNAKE"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := corestrings.ToCamelCase(tt.input); got != tt.want {
				t.Errorf("ToCamelCase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestScreamingSnakeToCamelCase(t *testing.T) {
	got := corestrings.ScreamingSnakeToCamelCase("hello_world")
	if got != "helloWorld" {
		t.Errorf("ScreamingSnakeToCamelCase(hello_world) = %q, want helloWorld", got)
	}
}
