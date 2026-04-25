// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings_test

import (
	"testing"

	"github.com/stretchr/testify/require"

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
			require.Equal(t, tt.want, corestrings.ToScreamingSnakeCase(tt.input))
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
			require.Equal(t, tt.want, corestrings.ToSnakeCase(tt.input))
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
			require.Equal(t, tt.want, corestrings.ToCamelCase(tt.input))
		})
	}
}

func TestScreamingSnakeToCamelCase(t *testing.T) {
	require.Equal(t, "helloWorld", corestrings.ScreamingSnakeToCamelCase("hello_world"))
}
