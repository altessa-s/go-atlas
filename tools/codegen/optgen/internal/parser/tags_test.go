// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package parser_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/parser"
)

func TestParseTag(t *testing.T) {
	t.Parallel()

	tagStr := "`opt:\"Name\" optgen:\"default=1\"`"

	require.Equal(t, "Name", parser.ParseTag(tagStr, "opt"))
	require.Equal(t, "default=1", parser.ParseTag(tagStr, "optgen"))
	require.Empty(t, parser.ParseTag(tagStr, "json"))
	require.Empty(t, parser.ParseTag("", "opt"))
	// Works without surrounding backticks too.
	require.Equal(t, "Name", parser.ParseTag(`opt:"Name"`, "opt"))
}

func TestLookupTag(t *testing.T) {
	t.Parallel()

	value, ok := parser.LookupTag("`opt:\"Name\"`", "opt")
	require.True(t, ok)
	require.Equal(t, "Name", value)

	// An empty tag value is distinguishable from a missing tag.
	value, ok = parser.LookupTag("`opt:\"\"`", "opt")
	require.True(t, ok)
	require.Empty(t, value)

	_, ok = parser.LookupTag("`opt:\"Name\"`", "json")
	require.False(t, ok)
}

func TestParseOptName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tag      string
		wantName string
		wantSkip bool
	}{
		{tag: "Name", wantName: "Name"},
		{tag: " Name ", wantName: "Name"},
		{tag: "-", wantSkip: true},
		{tag: " - ", wantSkip: true},
		{tag: ""},
	}

	for _, tc := range tests {
		t.Run(tc.tag, func(t *testing.T) {
			t.Parallel()

			name, skip := parser.ParseOptName(tc.tag)
			require.Equal(t, tc.wantName, name)
			require.Equal(t, tc.wantSkip, skip)
		})
	}
}

func TestParseExprList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "empty", input: "", want: nil},
		{name: "blank items", input: " , , ", want: nil},
		{name: "plain list", input: "a,b,c", want: []string{"a", "b", "c"}},
		{name: "bracketed list", input: "[a, b, c]", want: []string{"a", "b", "c"}},
		{name: "single item", input: "lower", want: []string{"lower"}},
		{
			name:  "nested brackets are not split",
			input: "a,[b,c],d",
			want:  []string{"a", "[b,c]", "d"},
		},
		{
			name:  "unclosed bracket keeps input intact",
			input: "[a,b",
			want:  []string{"[a,b"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, parser.ParseExprList(tc.input))
		})
	}
}

func TestParseOptModifiers(t *testing.T) {
	t.Parallel()

	require.Equal(t, []string{"lower", "trimspaces"}, parser.ParseOptModifiers("[lower, trimspaces]"))
	require.Equal(t, []string{"lower"}, parser.ParseOptModifiers("lower"))
	require.Nil(t, parser.ParseOptModifiers(""))
}

func TestParseOptGen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantDefault string
		wantMeta    map[string]string
	}{
		{name: "empty", input: ""},
		{name: "default only", input: "default=10", wantDefault: "10"},
		{
			name:        "default with metadata",
			input:       "append,default=x,err=boom",
			wantDefault: "x",
			wantMeta:    map[string]string{"append": "true", "err": "boom"},
		},
		{
			name:        "default with braces is not split",
			input:       "default=map[string]int{1: 2},notnil",
			wantDefault: "map[string]int{1: 2}",
			wantMeta:    map[string]string{"notnil": "true"},
		},
		{
			name:        "last default wins",
			input:       "default=a,default=b",
			wantDefault: "b",
		},
		{name: "bare flag", input: "manual", wantMeta: map[string]string{"manual": "true"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			defaultVal, meta := parser.ParseOptGen(tc.input)
			require.Equal(t, tc.wantDefault, defaultVal)
			require.Equal(t, tc.wantMeta, meta)
		})
	}
}
