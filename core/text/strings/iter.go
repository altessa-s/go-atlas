// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings

import (
	"iter"
	"strings"
)

// SplitSeq returns an [iter.Seq] that lazily yields substrings of s according
// to the supplied [SplitOptions]. It applies the same splitting semantics as
// [Split] (separator, case sensitivity, trimming, empty-element skipping, and
// max-split limiting) but avoids allocating an intermediate slice, making it
// suitable for large inputs or when only a prefix of the results is needed.
//
// When opts.Separator is the empty string, each UTF-8 rune is yielded
// individually. If the caller stops iterating early, no further work is done.
func SplitSeq(s string, opts SplitOptions) iter.Seq[string] {
	return func(yield func(string) bool) {
		if s == "" {
			if !opts.SkipEmpty {
				yield("")
			}
			return
		}

		separator := opts.Separator
		maxSplits := opts.MaxSplits
		numSplits := 0

		// Handle case-insensitive splitting
		if !opts.CaseSensitive && separator != "" {
			lowerS := strings.ToLower(s)
			lowerSep := strings.ToLower(separator)
			start := 0

			for {
				if maxSplits > 0 && numSplits >= maxSplits {
					if !yieldPart(s[start:], opts, yield) {
						return
					}
					return
				}

				idx := strings.Index(lowerS[start:], lowerSep)
				if idx == -1 {
					if !yieldPart(s[start:], opts, yield) {
						return
					}
					return
				}

				actualIdx := start + idx
				if !yieldPart(s[start:actualIdx], opts, yield) {
					return
				}
				numSplits++
				start = actualIdx + len(separator)
			}
		} else {
			// Case-sensitive splitting
			start := 0
			for {
				if maxSplits > 0 && numSplits >= maxSplits {
					if !yieldPart(s[start:], opts, yield) {
						return
					}
					return
				}

				if separator == "" {
					// Split by rune if separator is empty
					for _, r := range s[start:] {
						if !yieldPart(string(r), opts, yield) {
							return
						}
					}
					return
				}

				idx := strings.Index(s[start:], separator)
				if idx == -1 {
					if !yieldPart(s[start:], opts, yield) {
						return
					}
					return
				}

				actualIdx := start + idx
				if !yieldPart(s[start:actualIdx], opts, yield) {
					return
				}
				numSplits++
				start = actualIdx + len(separator)
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
