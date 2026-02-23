// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings_test

import (
	"slices"
	"strings"
	"testing"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

func TestIsEmptyOrWhitespace(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"", true},
		{"   ", true},
		{"\t\n", true},
		{"hello", false},
		{" a ", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := corestrings.IsEmptyOrWhitespace(tt.input); got != tt.want {
				t.Errorf("IsEmptyOrWhitespace(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestTrimPrefixFast(t *testing.T) {
	tests := []struct {
		s, prefix, want string
	}{
		{"hello-world", "hello-", "world"},
		{"hello", "xyz", "hello"},
		{"", "x", ""},
	}
	for _, tt := range tests {
		if got := corestrings.TrimPrefixFast(tt.s, tt.prefix); got != tt.want {
			t.Errorf("TrimPrefixFast(%q, %q) = %q, want %q", tt.s, tt.prefix, got, tt.want)
		}
	}
}

func TestSplitSeq(t *testing.T) {
	opts := corestrings.SplitOptions{Separator: ",", TrimSpace: true, SkipEmpty: true}
	var result []string
	for v := range corestrings.SplitSeq("a, b, , c", opts) {
		result = append(result, v)
	}
	want := []string{"a", "b", "c"}
	if !slices.Equal(result, want) {
		t.Errorf("SplitSeq() = %v, want %v", result, want)
	}
}

func TestGetStringSlice(t *testing.T) {
	s := corestrings.GetStringSlice()
	if s == nil {
		t.Fatal("GetStringSlice() returned nil")
	}
	*s = append(*s, "a", "b")
	corestrings.PutStringSlice(s)
}

func TestGetStringSliceWithCapacity(t *testing.T) {
	s := corestrings.GetStringSliceWithCapacity(50)
	if s == nil {
		t.Fatal("GetStringSliceWithCapacity() returned nil")
	}
	if cap(*s) < 50 {
		t.Errorf("capacity = %d, want >= 50", cap(*s))
	}
	corestrings.PutStringSlice(s)
}

func TestBuildString(t *testing.T) {
	got := corestrings.BuildString(func(b *strings.Builder) {
		b.WriteString("hello")
		b.WriteString(" world")
	})
	if got != "hello world" {
		t.Errorf("BuildString() = %q, want 'hello world'", got)
	}
}
