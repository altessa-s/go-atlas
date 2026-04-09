// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings_test

import (
	"testing"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// TestSubstringMatch_ShorterHaystackAfterPaddingRemoval confirms the fix that
// dropped the misleading null-byte padding: when the haystack is shorter
// than the needle, the match must return false directly rather than padding
// the haystack and inadvertently matching trailing null bytes if the caller
// ever constructed such a needle.
func TestSubstringMatch_ShorterHaystackAfterPaddingRemoval(t *testing.T) {
	// Regular case: haystack shorter, no match expected.
	if corestrings.SubstringMatch("Hi", "Hello") {
		t.Error(`SubstringMatch("Hi", "Hello") = true, want false`)
	}

	// Previously, padding "Hi" with three null bytes could let a needle
	// containing a trailing "\x00" spuriously match. After removing the
	// padding, this can no longer happen.
	if corestrings.SubstringMatch("Hi", "hi\x00\x00\x00") {
		t.Error(`SubstringMatch("Hi", "hi\x00\x00\x00") = true, want false (padding removed)`)
	}
}

// BenchmarkSubstringMatch measures the common match path. After removing the
// dead null-padding branch the happy path should be allocation-free.
func BenchmarkSubstringMatch(b *testing.B) {
	const (
		haystack = "The quick brown fox jumps over the lazy dog"
		needle   = "BROWN"
	)
	b.ReportAllocs()
	for b.Loop() {
		_ = corestrings.SubstringMatch(haystack, needle)
	}
}

// BenchmarkSubstringMatch_ShorterHaystack covers the short-input early
// return path that previously allocated a null-padded copy of the haystack.
// Should now be allocation-free.
func BenchmarkSubstringMatch_ShorterHaystack(b *testing.B) {
	const (
		haystack = "Hi"
		needle   = "Hello, world!"
	)
	b.ReportAllocs()
	for b.Loop() {
		_ = corestrings.SubstringMatch(haystack, needle)
	}
}
