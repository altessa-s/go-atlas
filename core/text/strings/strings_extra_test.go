// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings_test

import (
	"testing"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

func TestSubstringMatch_NotFound(t *testing.T) {
	if corestrings.SubstringMatch("Hello", "xyz") {
		t.Error("should not match")
	}
}

func TestSubstringMatch_ShorterString(t *testing.T) {
	if corestrings.SubstringMatch("Hi", "Hello") {
		t.Error("shorter string should not match longer substr")
	}
}

func TestSubstringMatch_Empty(t *testing.T) {
	if !corestrings.SubstringMatch("anything", "") {
		t.Error("empty substr should always match")
	}
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
			if got := corestrings.TimingSafeSubstringMatch(tt.s, tt.substr); got != tt.want {
				t.Errorf("TimingSafeSubstringMatch(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

func TestTimingSafePrefixMatch_ShorterString(t *testing.T) {
	if corestrings.TimingSafePrefixMatch("Hi", "Hello") {
		t.Error("shorter string should not match longer prefix")
	}
}

func TestTimingSafePrefixMatch_ExactMatch(t *testing.T) {
	if !corestrings.TimingSafePrefixMatch("bearer", "bearer") {
		t.Error("exact match should succeed")
	}
}

func TestToPtr_EmptyString(t *testing.T) {
	p := corestrings.ToPtr("")
	if p != nil {
		t.Error("ToPtr('') should return nil")
	}
}

func TestToPtr_NonEmpty(t *testing.T) {
	p := corestrings.ToPtr("hello")
	if p == nil || *p != "hello" {
		t.Error("ToPtr('hello') should return pointer to 'hello'")
	}
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
		got := corestrings.IsTrimmed(tt.input)
		if got != tt.want {
			t.Errorf("IsTrimmed(%q) = %v, want %v", tt.input, got, tt.want)
		}
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
		got := corestrings.IsTrimmedUnsafe(tt.input)
		if got != tt.want {
			t.Errorf("IsTrimmedUnsafe(%q) = %v, want %v", tt.input, got, tt.want)
		}
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
		got := corestrings.TrimSuffixFast(tt.s, tt.suffix)
		if got != tt.want {
			t.Errorf("TrimSuffixFast(%q, %q) = %q, want %q", tt.s, tt.suffix, got, tt.want)
		}
	}
}

func TestStringEqualsUnsafe(t *testing.T) {
	if !corestrings.StringEqualsUnsafe("hello", "hello") {
		t.Error("same string should be equal")
	}
	if corestrings.StringEqualsUnsafe("hi", "hello") {
		t.Error("different length should not be equal")
	}
	if corestrings.StringEqualsUnsafe("abc", "xyz") {
		t.Error("different strings should not be equal")
	}
}

func TestSplitSeq_CaseInsensitive(t *testing.T) {
	opts := corestrings.SplitOptions{Separator: "SEP", CaseSensitive: false}
	count := 0
	for range corestrings.SplitSeq("asepbSEPc", opts) {
		count++
	}
	if count != 3 {
		t.Errorf("SplitSeq case-insensitive count = %d, want 3", count)
	}
}

func TestGetStringBuilder(t *testing.T) {
	sb := corestrings.GetStringBuilder()
	if sb == nil {
		t.Fatal("GetStringBuilder returned nil")
	}
	sb.WriteString("test")
	corestrings.PutStringBuilder(sb)
}

func TestSecureString_Len(t *testing.T) {
	ss := corestrings.NewSecureString("hello")
	if ss.Len() != 5 {
		t.Errorf("Len() = %d, want 5", ss.Len())
	}
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
		got := corestrings.ToScreamingSnakeCase(tt.input)
		if got != tt.want {
			t.Errorf("ToScreamingSnakeCase(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
