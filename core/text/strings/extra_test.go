// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

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
			require.Equal(t, tt.want, corestrings.IsEmptyOrWhitespace(tt.input))
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
		require.Equal(t, tt.want, corestrings.TrimPrefixFast(tt.s, tt.prefix))
	}
}

func TestSplitSeq(t *testing.T) {
	opts := corestrings.SplitOptions{Separator: ",", TrimSpace: true, SkipEmpty: true}
	var result []string
	for v := range corestrings.SplitSeq("a, b, , c", opts) {
		result = append(result, v)
	}
	want := []string{"a", "b", "c"}
	require.Equal(t, want, result)
}

func TestGetStringSlice(t *testing.T) {
	s := corestrings.GetStringSlice()
	require.NotNil(t, s)
	*s = append(*s, "a", "b")
	corestrings.PutStringSlice(s)
}

func TestGetStringSliceWithCapacity(t *testing.T) {
	s := corestrings.GetStringSliceWithCapacity(50)
	require.NotNil(t, s)
	require.GreaterOrEqual(t, cap(*s), 50)
	corestrings.PutStringSlice(s)
}

func TestBuildString(t *testing.T) {
	got := corestrings.BuildString(func(b *strings.Builder) {
		b.WriteString("hello")
		b.WriteString(" world")
	})
	require.Equal(t, "hello world", got)
}
