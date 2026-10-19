// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency"
)

// FuzzComposeKey checks that ComposeKey is injective: every composed key
// decomposes back into exactly its tuple, and two tuples share a key only
// when they are equal — including tuples of different lengths.
func FuzzComposeKey(f *testing.F) {
	seeds := [][4]string{
		{"a:b", "c", "a", "b:c"},
		{"", "a", "a", ""},
		{"namespace", "", "namespace", ":"},
		{"1:a", "", "a", ""},
		{"κλειδί", "\x00\xff", "idk:v2:", "0:"},
	}
	for _, s := range seeds {
		f.Add(s[0], s[1], s[2], s[3])
	}

	f.Fuzz(func(t *testing.T, a, b, c, d string) {
		tuples := [][]string{{a, b}, {c, d}, {a, b, c}, {a + b}, {c}}
		keys := make([]string, len(tuples))
		for i, tuple := range tuples {
			keys[i] = idempotency.ComposeKey(tuple...)
			got, ok := decomposeKey(keys[i])
			require.True(t, ok, "ComposeKey(%q) = %q does not decompose", tuple, keys[i])
			require.Equal(t, tuple, got, "ComposeKey(%q) = %q", tuple, keys[i])
		}
		for i := range tuples {
			for j := i + 1; j < len(tuples); j++ {
				require.Equal(t, slices.Equal(tuples[i], tuples[j]), keys[i] == keys[j],
					"tuples %q and %q compose to %q and %q", tuples[i], tuples[j], keys[i], keys[j])
			}
		}
	})
}
