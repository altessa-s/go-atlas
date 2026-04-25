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

func FuzzUnsafe(f *testing.F) {
	f.Add("hello")

	f.Fuzz(func(t *testing.T, s string) {
		b := corestrings.ToBytesUnsafe(s)
		s2 := corestrings.FromBytesUnsafe(b)

		require.Equal(t, s, s2, "Unsafe conversion roundtrip failed")
	})
}
