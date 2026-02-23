// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings_test

import (
	"testing"
	"unsafe"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

func TestIsEmpty(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  bool
	}{
		{"empty string", "", true},
		{"whitespace string", "   ", true},
		{"non-empty string", "test", false},
		{"nil ptr", (*string)(nil), true},
		{"ptr to empty", func() *string { s := ""; return &s }(), true},
		{"ptr to whitespace", func() *string { s := "  "; return &s }(), true},
		{"ptr to non-empty", func() *string { s := "test"; return &s }(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bool
			switch v := tt.input.(type) {
			case string:
				got = corestrings.IsEmpty(v)
			case *string:
				got = corestrings.IsEmpty(v)
			}
			if got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPtrConversions(t *testing.T) {
	t.Run("ToPtr", func(t *testing.T) {
		s := "test"
		if p := corestrings.ToPtr(s); p == nil || *p != "test" {
			t.Error("ToPtr(test) failed")
		}
		if corestrings.ToPtr("") != nil {
			t.Error("ToPtr(\"\") should return nil")
		}
		if corestrings.ToPtr("  ") != nil {
			t.Error("ToPtr(\"  \") should return nil")
		}
	})

	t.Run("FromPtr", func(t *testing.T) {
		s := "test"
		if corestrings.FromPtr(&s) != "test" {
			t.Error("FromPtr(&test) failed")
		}
		if corestrings.FromPtr(nil) != "" {
			t.Error("FromPtr(nil) should return empty string")
		}
	})
}

func TestTo(t *testing.T) {
	t.Run("Integers", func(t *testing.T) {
		if corestrings.ToInt("42") != 42 {
			t.Error("ToInt failed")
		}
		if corestrings.To[int8]("127") != 127 {
			t.Error("To[int8] failed")
		}
		if corestrings.To[int]("invalid") != 0 {
			t.Error("To[int](invalid) should be 0")
		}
	})

	t.Run("Floats", func(t *testing.T) {
		if corestrings.ToFloat64("3.14") != 3.14 {
			t.Error("ToFloat64 failed")
		}
		if corestrings.To[float32]("invalid") != 0 {
			t.Error("To[float32](invalid) should be 0")
		}
	})
}

func TestJoin(t *testing.T) {
	tests := []struct {
		name     string
		elements []string
		opts     corestrings.JoinOptions
		want     string
	}{
		{
			name:     "basic join",
			elements: []string{"a", "b", "c"},
			opts:     corestrings.JoinOptions{Separator: ","},
			want:     "a,b,c",
		},
		{
			name:     "skip empty",
			elements: []string{"a", "", "b"},
			opts:     corestrings.JoinOptions{Separator: ",", SkipEmpty: true},
			want:     "a,b",
		},
		{
			name:     "with prefix suffix",
			elements: []string{"a", "b"},
			opts:     corestrings.JoinOptions{Separator: ",", Prefix: "(", Suffix: ")"},
			want:     "(a,b)",
		},
		{
			name:     "empty result",
			elements: []string{},
			opts:     corestrings.JoinOptions{},
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := corestrings.Join(tt.elements, tt.opts); got != tt.want {
				t.Errorf("Join() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSplit(t *testing.T) {
	tests := []struct {
		name string
		s    string
		opts corestrings.SplitOptions
		want []string
	}{
		{
			name: "basic split",
			s:    "a,b,c",
			opts: corestrings.SplitOptions{Separator: ","},
			want: []string{"a", "b", "c"},
		},
		{
			name: "skip empty parts",
			s:    "a,,b",
			opts: corestrings.SplitOptions{Separator: ",", SkipEmpty: true},
			want: []string{"a", "b"},
		},
		{
			name: "trim space",
			s:    "a , b ",
			opts: corestrings.SplitOptions{Separator: ",", TrimSpace: true},
			want: []string{"a", "b"},
		},
		{
			name: "case insensitive",
			s:    "aANDb",
			opts: corestrings.SplitOptions{Separator: "and", CaseSensitive: false},
			want: []string{"a", "b"},
		},
		{
			name: "max splits",
			s:    "a,b,c",
			opts: corestrings.SplitOptions{Separator: ",", MaxSplits: 1},
			want: []string{"a", "b,c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := corestrings.Split(tt.s, tt.opts)
			if len(got) != len(tt.want) {
				t.Errorf("Split() len = %d, want %d (%v)", len(got), len(tt.want), got)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Split()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestContains(t *testing.T) {
	t.Run("Case Insensitive", func(t *testing.T) {
		res := corestrings.Contains("Hello World", "hello", corestrings.ContainsOptions{CaseSensitive: false})
		if !res.Found {
			t.Error("Should find 'hello' in 'Hello World' (case-insensitive)")
		}
	})

	t.Run("Whole Words", func(t *testing.T) {
		if corestrings.Contains("hello world", "hell", corestrings.ContainsOptions{MatchWholeWords: true}).Found {
			t.Error("Should NOT find 'hell' in 'hello world' as whole word")
		}
		if !corestrings.Contains("hello world", "hello", corestrings.ContainsOptions{MatchWholeWords: true}).Found {
			t.Error("Should find 'hello' in 'hello world' as whole word")
		}
	})

	t.Run("Count", func(t *testing.T) {
		res := corestrings.Contains("test test", "test", corestrings.ContainsOptions{Count: true})
		if res.Count != 2 {
			t.Errorf("Count = %d, want 2", res.Count)
		}
		if len(res.Positions) != 2 || res.Positions[0] != 0 || res.Positions[1] != 5 {
			t.Errorf("Positions = %v, want [0 5]", res.Positions)
		}
	})
}

func TestSafeComparisons(t *testing.T) {
	if !corestrings.SecureCompare("secret", "secret") {
		t.Error("SecureCompare failed for equal strings")
	}
	if corestrings.SecureCompare("secret", "wrong") {
		t.Error("SecureCompare passed for different strings")
	}

	if !corestrings.TimingSafePrefixMatch("Bearer token", "bearer") {
		t.Error("TimingSafePrefixMatch failed")
	}

	if !corestrings.SubstringMatch("Hello World", "world") {
		t.Error("SubstringMatch failed")
	}
}

func TestUnsafe(t *testing.T) {
	t.Run("ToBytesUnsafe", func(t *testing.T) {
		s := "hello"
		b := corestrings.ToBytesUnsafe(s)
		if string(b) != s {
			t.Error("ToBytesUnsafe content mismatch")
		}
		// Verify zero-copy by checking address (careful with GC moving things, but basic check)
		// We expect b to point to s data.
		sData := unsafe.StringData(s)
		bData := unsafe.SliceData(b)
		if sData != bData {
			t.Error("ToBytesUnsafe did not return zero-copy slice")
		}
	})

	t.Run("FromBytesUnsafe", func(t *testing.T) {
		b := []byte("hello")
		s := corestrings.FromBytesUnsafe(b)
		if s != "hello" {
			t.Error("FromBytesUnsafe content mismatch")
		}
	})

	t.Run("StringEqualsUnsafe", func(t *testing.T) {
		if !corestrings.StringEqualsUnsafe("a", "a") {
			t.Error("StringEqualsUnsafe failed for equal")
		}
		if corestrings.StringEqualsUnsafe("a", "b") {
			t.Error("StringEqualsUnsafe passed for different")
		}
	})
}

func TestConcat(t *testing.T) {
	if got := corestrings.Concat("a", "b", "c"); got != "abc" {
		t.Errorf("Concat() = %v, want abc", got)
	}
	if got := corestrings.ConcatUnsafe("a", "b"); got != "ab" {
		t.Errorf("ConcatUnsafe() = %v, want ab", got)
	}
}

func TestCase(t *testing.T) {
	if !corestrings.IsLowercase("abc") || corestrings.IsLowercase("Abc") {
		t.Error("IsLowercase failed")
	}
	if !corestrings.IsUppercase("ABC") || corestrings.IsUppercase("Abc") {
		t.Error("IsUppercase failed")
	}

	if !corestrings.IsLowercaseUnsafe("abc") || corestrings.IsLowercaseUnsafe("Abc") {
		t.Error("IsLowercaseUnsafe failed")
	}
	if !corestrings.IsUppercaseUnsafe("ABC") || corestrings.IsUppercaseUnsafe("Abc") {
		t.Error("IsUppercaseUnsafe failed")
	}
}

func TestTrim(t *testing.T) {
	t.Run("IsTrimmed", func(t *testing.T) {
		if !corestrings.IsTrimmed("abc") {
			t.Error("abc should be trimmed")
		}
		if corestrings.IsTrimmed(" abc") {
			t.Error("' abc' should not be trimmed")
		}
		if !corestrings.IsTrimmed("") {
			t.Error("empty string should be trimmed")
		}
	})

	t.Run("IsTrimmedUnsafe", func(t *testing.T) {
		if !corestrings.IsTrimmedUnsafe("abc") {
			t.Error("IsTrimmedUnsafe(abc) failed")
		}
		if corestrings.IsTrimmedUnsafe(" abc") {
			t.Error("IsTrimmedUnsafe(' abc') failed")
		}
	})

	t.Run("TrimSuffixFast", func(t *testing.T) {
		if corestrings.TrimSuffixFast("file.go", ".go") != "file" {
			t.Error("TrimSuffixFast failed to trim")
		}
		if corestrings.TrimSuffixFast("file.go", ".txt") != "file.go" {
			t.Error("TrimSuffixFast should not trim mismatch")
		}
	})
}
