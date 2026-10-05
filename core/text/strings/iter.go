// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings

import (
	"iter"
	"strings"
)

// SplitSeq returns an [iter.Seq] that lazily yields substrings of s according
// to the supplied [SplitOptions]. It yields exactly the elements [Split]
// returns (separator, case-insensitive matching rules, trimming,
// empty-element skipping, and max-split limiting) but avoids allocating an
// intermediate slice, making it suitable for large inputs or when only a
// prefix of the results is needed.
//
// When opts.Separator is the empty string, s is split after each UTF-8
// sequence, as [strings.Split] does. If the caller stops iterating early, no
// further work is done.
func SplitSeq(s string, opts SplitOptions) iter.Seq[string] {
	return func(yield func(string) bool) {
		if s == "" {
			if !opts.SkipEmpty {
				yield("")
			}
			return
		}

		separator := opts.Separator
		if separator == "" {
			splitRunes(s, opts.MaxSplits, func(part string) bool { return yieldPart(part, opts, yield) })
			return
		}

		var sp splitter
		if opts.CaseSensitive {
			sp = exactSplitter(s, separator, opts.MaxSplits)
		} else {
			sp = caseInsensitiveSplitter(s, separator, opts.MaxSplits)
		}
		for part, ok := sp.next(); ok; part, ok = sp.next() {
			if !yieldPart(part, opts, yield) {
				return
			}
		}
	}
}

func yieldPart(part string, opts SplitOptions, yield func(string) bool) bool {
	if opts.TrimSpace {
		part = strings.TrimSpace(part)
	}
	if opts.SkipEmpty && part == "" {
		return true
	}
	return yield(part)
}
