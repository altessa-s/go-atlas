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
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"hello_world", "helloWorld"},
		{"simple", "simple"},
		{"", ""},
		{"SCREAMING_SNAKE", "sCREAMINGSNAKE"},
		{"space delimited words", "spaceDelimitedWords"},
		{"__leading_and__double", "leadingAndDouble"},
		{"trailing_", "trailing"},
		{"Ärger_über_öl", "ärgerÜberÖl"},
		{"über_größe", "überGröße"},
		{"日本_語", "日本語"},
		{"привет_мир", "приветМир"},
		{"ÉCOLE_ÉTÉ", "éCOLEÉTÉ"},
		{"a\u00a0b", "aB"},
		{"x_\xffy", "x\xffy"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, corestrings.ToCamelCase(tt.input))
		})
	}
}

func TestScreamingSnakeToCamelCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"HELLO", "hello"},
		{"HELLO_WORLD", "helloWorld"},
		{"hello_world", "helloWorld"},
		{"Hello_World", "helloWorld"},
		{"REAL_IP", "realIp"},
		{"MAX_RETRY_COUNT", "maxRetryCount"},
		{"HTTP2_PORT", "http2Port"},
		{"_LEADING__DOUBLE_", "leadingDouble"},
		{"ÉCOLE_ÉTÉ", "écoleÉté"},
		{"ПРИВЕТ_МИР", "приветМир"},
		{"ÜBER_GRÖSSE", "überGrösse"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, corestrings.ScreamingSnakeToCamelCase(tt.input))
		})
	}
}

// TestCamelCase_ASCIIAndUnicodePathsAgree checks that the ASCII fast path and
// the rune-based path produce the same result for ASCII input: appending a
// non-ASCII suffix forces the rune path and must not change the ASCII prefix.
func TestCamelCase_ASCIIAndUnicodePathsAgree(t *testing.T) {
	t.Parallel()

	inputs := []string{"hello_world", "HELLO_WORLD", "a b\tc", "__x__y__", "MiXeD_cAsE", "x1_y2_z3", "trailing_"}
	const suffix = "_é"
	for _, in := range inputs {
		require.Equal(t, corestrings.ToCamelCase(in)+"É", corestrings.ToCamelCase(in+suffix), "ToCamelCase(%q)", in)
		require.Equal(t, corestrings.ScreamingSnakeToCamelCase(in)+"É",
			corestrings.ScreamingSnakeToCamelCase(in+suffix), "ScreamingSnakeToCamelCase(%q)", in)
	}
}
