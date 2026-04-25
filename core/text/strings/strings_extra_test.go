// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

func TestSubstringMatch_NotFound(t *testing.T) {
	require.False(t, corestrings.SubstringMatch("Hello", "xyz"), "should not match")
}

func TestSubstringMatch_ShorterString(t *testing.T) {
	require.False(t, corestrings.SubstringMatch("Hi", "Hello"), "shorter string should not match longer substr")
}

func TestSubstringMatch_Empty(t *testing.T) {
	require.True(t, corestrings.SubstringMatch("anything", ""), "empty substr should always match")
}

func TestTimingSafeSubstringMatch(t *testing.T) {
	tests := []struct {
		name      string
		s, substr string
		want      bool
	}{
		{"found", "Hello World", "world", true},
		{"not found", "Hello", "xyz", false},
		{"empty substr", "anything", "", true},
		{"shorter string", "Hi", "Hello", false},
		{"exact match", "hello", "hello", true},
		{"case insensitive", "ABC", "abc", true},
		{"at start", "foobar", "foo", true},
		{"at end", "foobar", "bar", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, corestrings.TimingSafeSubstringMatch(tt.s, tt.substr))
		})
	}
}

func TestTimingSafePrefixMatch_ShorterString(t *testing.T) {
	require.False(t, corestrings.TimingSafePrefixMatch("Hi", "Hello"), "shorter string should not match longer prefix")
}

func TestTimingSafePrefixMatch_ExactMatch(t *testing.T) {
	require.True(t, corestrings.TimingSafePrefixMatch("bearer", "bearer"), "exact match should succeed")
}

func TestToPtr_EmptyString(t *testing.T) {
	require.Nil(t, corestrings.ToPtr(""))
}

func TestToPtr_NonEmpty(t *testing.T) {
	p := corestrings.ToPtr("hello")
	require.NotNil(t, p)
	require.Equal(t, "hello", *p)
}

func TestIsTrimmed(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"hello", true},
		{" hello", false},
		{"hello ", false},
		{" hello ", false},
		{"", true},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, corestrings.IsTrimmed(tt.input))
	}
}

func TestIsTrimmedUnsafe(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"hello", true},
		{" hello", false},
		{"hello ", false},
		{"", true},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, corestrings.IsTrimmedUnsafe(tt.input))
	}
}

func TestTrimSuffixFast(t *testing.T) {
	tests := []struct {
		s, suffix, want string
	}{
		{"hello.go", ".go", "hello"},
		{"hello", ".go", "hello"},
		{".go", ".go", ""},
		{"", ".go", ""},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, corestrings.TrimSuffixFast(tt.s, tt.suffix))
	}
}

func TestStringEqualsUnsafe(t *testing.T) {
	require.True(t, corestrings.StringEqualsUnsafe("hello", "hello"), "same string should be equal")
	require.False(t, corestrings.StringEqualsUnsafe("hi", "hello"), "different length should not be equal")
	require.False(t, corestrings.StringEqualsUnsafe("abc", "xyz"), "different strings should not be equal")
}

func TestSplitSeq_CaseInsensitive(t *testing.T) {
	opts := corestrings.SplitOptions{Separator: "SEP", CaseSensitive: false}
	count := 0
	for range corestrings.SplitSeq("asepbSEPc", opts) {
		count++
	}
	require.Equal(t, 3, count)
}

func TestGetStringBuilder(t *testing.T) {
	sb := corestrings.GetStringBuilder()
	require.NotNil(t, sb)
	sb.WriteString("test")
	corestrings.PutStringBuilder(sb)
}

func TestSecureString_Len(t *testing.T) {
	ss := corestrings.NewSecureString("hello")
	require.Equal(t, 5, ss.Len())
}

func TestToScreamingSnakeCase_Complex(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"camelCase", "CAMEL_CASE"},
		{"HTTPServer", "HTTP_SERVER"},
		{"simpleTest", "SIMPLE_TEST"},
		{"", ""},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, corestrings.ToScreamingSnakeCase(tt.input))
	}
}
