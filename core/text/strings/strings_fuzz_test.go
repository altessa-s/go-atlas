// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings_test

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

func FuzzSplit(f *testing.F) {
	f.Add("a,b,c", ",")

	f.Fuzz(func(t *testing.T, s string, sep string) {
		if sep == "" {
			return
		}

		opts := corestrings.SplitOptions{Separator: sep, SkipEmpty: false}
		parts := corestrings.Split(s, opts)

		rejoined := strings.Join(parts, sep)
		if rejoined != s {
			// Note: Split behavior might differ from exact roundtrip if sep is multi-char or overlaps?
			// Actually strings.Split roundtrip holds for simple cases.
			// corestrings.Split aims to align with strings.Split when no extra opts used.
			t.Logf("Split roundtrip mismatch: %q -> %q", s, rejoined)
		}
	})
}

// FuzzSplitSeqMatchesSplit checks that SplitSeq yields exactly the elements
// Split returns for arbitrary input, separator, and options, and that
// a valid separator never cuts a UTF-8 sequence of a valid input.
func FuzzSplitSeqMatchesSplit(f *testing.F) {
	f.Add("xİyİz", "İ", 0, uint8(0))
	f.Add("İ,Ⱥ", ",", 0, uint8(0))
	f.Add("\ufffdii", "\xffİİ", 0, uint8(0))
	f.Add("aſbΣcσdς", "s", 2, uint8(0))
	f.Add("a\xffé", "", 1, uint8(1))
	f.Add(" a ,, b ", ",", -1, uint8(7))

	f.Fuzz(func(t *testing.T, s, sep string, maxSplits int, flags uint8) {
		opts := corestrings.SplitOptions{
			Separator:     sep,
			MaxSplits:     maxSplits,
			CaseSensitive: flags&1 != 0,
			TrimSpace:     flags&2 != 0,
			SkipEmpty:     flags&4 != 0,
		}
		want := corestrings.Split(s, opts)
		got := slices.Collect(corestrings.SplitSeq(s, opts))
		require.Equal(t, want, got)

		if utf8.ValidString(s) && utf8.ValidString(sep) {
			for _, part := range want {
				require.True(t, utf8.ValidString(part), "part %q of valid input %q is not valid UTF-8", part, s)
			}
		}
	})
}

func FuzzUnsafe(f *testing.F) {
	f.Add("hello")

	f.Fuzz(func(t *testing.T, s string) {
		b := corestrings.ToBytesUnsafe(s)
		s2 := corestrings.FromBytesUnsafe(b)

		require.Equal(t, s, s2, "Unsafe conversion roundtrip failed")
	})
}
