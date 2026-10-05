// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings_test

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// splitSeqTokens is the alphabet the property test draws inputs and
// separators from. It mixes ASCII, runes whose lowercase form has a different
// UTF-8 width ("İ" and the Kelvin sign shrink, "Ⱥ" grows), runes that only fold under
// simple case folding ("ſ"), multi-byte runes, whitespace, and invalid UTF-8.
var splitSeqTokens = []string{
	"a", "A", "b", "B", ",", ";", " ", "\t", "x", "X",
	"İ", "i", "I", "\u0307", "\u212A", "k", "K", "ſ", "s", "S", "Ⱥ", "ⱥ",
	"é", "É", "ß", "ẞ", "日本", "\xff", "\xc3",
}

func randomSplitString(r *rand.Rand, maxTokens int) string {
	var b strings.Builder
	for range r.IntN(maxTokens + 1) {
		b.WriteString(splitSeqTokens[r.IntN(len(splitSeqTokens))])
	}
	return b.String()
}

func requireSplitSeqMatchesSplit(t testing.TB, s string, opts corestrings.SplitOptions) {
	t.Helper()
	want := corestrings.Split(s, opts)
	got := slices.Collect(corestrings.SplitSeq(s, opts))
	require.Equal(t, want, got, "s=%q opts=%+v", s, opts)
}

func TestSplitSeq_MatchesSplit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    string
		opts corestrings.SplitOptions
		want []string
	}{
		{
			name: "dotted capital I separator grows when lowercased",
			s:    "xİyİz",
			opts: corestrings.SplitOptions{Separator: "İ"},
			want: []string{"x", "y", "z"},
		},
		{
			name: "dotted capital I in input only",
			s:    "İa,İb",
			opts: corestrings.SplitOptions{Separator: ","},
			want: []string{"İa", "İb"},
		},
		{
			// Lowercasing shrinks "İ" (2 -> 1 byte) and grows "Ⱥ" (2 -> 3 bytes), so
			// the total length is unchanged but the offsets in between are not.
			name: "width changes cancel out across the input",
			s:    "İ,Ⱥ",
			opts: corestrings.SplitOptions{Separator: ","},
			want: []string{"İ", "Ⱥ"},
		},
		{
			name: "long s folds to s",
			s:    "aſb",
			opts: corestrings.SplitOptions{Separator: "s"},
			want: []string{"a", "b"},
		},
		{
			name: "long s folds to s regardless of other non-ASCII runes",
			s:    "İȺſ",
			opts: corestrings.SplitOptions{Separator: "s"},
			want: []string{"İȺ", ""},
		},
		{
			name: "kelvin sign folds to k",
			s:    "1\u212A2k3K4",
			opts: corestrings.SplitOptions{Separator: "k"},
			want: []string{"1", "2", "3", "4"},
		},
		{
			name: "greek sigma variants fold together",
			s:    "aΣbσcςd",
			opts: corestrings.SplitOptions{Separator: "σ"},
			want: []string{"a", "b", "c", "d"},
		},
		{
			name: "case-sensitive ignores folding",
			s:    "aſbsc",
			opts: corestrings.SplitOptions{Separator: "s", CaseSensitive: true},
			want: []string{"aſb", "c"},
		},
		{
			name: "invalid separator whose lowercase width cancels out",
			s:    "\ufffdii",
			opts: corestrings.SplitOptions{Separator: "\xffİİ"},
			want: []string{"\ufffdii"},
		},
		{
			name: "invalid separator on invalid input",
			s:    "A\xffİa\xffİ",
			opts: corestrings.SplitOptions{Separator: "\xffİ"},
			want: []string{"A", "a", ""},
		},
		{
			name: "case-insensitive ASCII separator",
			s:    "aSEPbsepc",
			opts: corestrings.SplitOptions{Separator: "sep"},
			want: []string{"a", "b", "c"},
		},
		{
			name: "case-insensitive with max splits",
			s:    "xİyİz",
			opts: corestrings.SplitOptions{Separator: "İ", MaxSplits: 1},
			want: []string{"x", "yİz"},
		},
		{
			name: "invalid UTF-8 separator falls back to exact match",
			s:    "a\xffb\xffc",
			opts: corestrings.SplitOptions{Separator: "\xff"},
			want: []string{"a", "b", "c"},
		},
		{
			name: "empty separator keeps invalid bytes intact",
			s:    "a\xffé",
			opts: corestrings.SplitOptions{CaseSensitive: true},
			want: []string{"a", "\xff", "é"},
		},
		{
			name: "empty separator honors max splits",
			s:    "abcd",
			opts: corestrings.SplitOptions{CaseSensitive: true, MaxSplits: 2},
			want: []string{"a", "b", "cd"},
		},
		{
			name: "empty separator with max splits equal to rune count",
			s:    "ab",
			opts: corestrings.SplitOptions{MaxSplits: 2},
			want: []string{"a", "b"},
		},
		{
			name: "trim and skip empty",
			s:    " a ,, b ",
			opts: corestrings.SplitOptions{Separator: ",", TrimSpace: true, SkipEmpty: true, CaseSensitive: true},
			want: []string{"a", "b"},
		},
		{
			name: "empty input skip empty",
			s:    "",
			opts: corestrings.SplitOptions{Separator: ",", SkipEmpty: true},
			want: nil,
		},
		{
			name: "empty input",
			s:    "",
			opts: corestrings.SplitOptions{Separator: ","},
			want: []string{""},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, corestrings.Split(tc.s, tc.opts))
			require.Equal(t, tc.want, slices.Collect(corestrings.SplitSeq(tc.s, tc.opts)))
		})
	}
}

func TestSplitSeq_PropertyMatchesSplit(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // deterministic test input, not security sensitive
	for range 20000 {
		s := randomSplitString(r, 12)
		opts := corestrings.SplitOptions{
			Separator:     randomSplitString(r, 2),
			MaxSplits:     r.IntN(5) - 1,
			TrimSpace:     r.IntN(2) == 0,
			SkipEmpty:     r.IntN(2) == 0,
			CaseSensitive: r.IntN(2) == 0,
		}
		requireSplitSeqMatchesSplit(t, s, opts)
	}
}

func TestSplitSeq_EarlyStop(t *testing.T) {
	t.Parallel()

	for _, opts := range []corestrings.SplitOptions{
		{Separator: ","},
		{Separator: ",", CaseSensitive: true},
		{Separator: "İ"},
		{},
	} {
		var got []string
		for part := range corestrings.SplitSeq("İ,a,İ,b", opts) {
			got = append(got, part)
			break
		}
		require.Len(t, got, 1, "opts=%+v", opts)
	}
}
