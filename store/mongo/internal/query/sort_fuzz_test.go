// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package query

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	corestrings "github.com/altessa-s/go-atlas-core/text/strings"
)

// FuzzParseSortStringStrict_HonorsAllowlist is the injection oracle for the
// sort parameter.
//
// Strict is the entry point documented for untrusted input: a REST query
// parameter, a gRPC request field. Two things must hold no matter what arrives
// — the result never names a field outside the allowlist, and it never names a
// `$`-prefixed operator key. The first bounds the query to indexed fields (an
// unindexed sort forces an in-memory sort that aborts past 32 MB); the second
// keeps `$natural` and friends out of a caller-supplied string.
func FuzzParseSortStringStrict_HonorsAllowlist(f *testing.F) {
	f.Add("name,-age", "name,age")
	f.Add("$natural", "name")
	f.Add("-$natural", "$natural") // Even allowlisting it must not let it through.
	f.Add("", "")
	f.Add(",,,,", "name")
	f.Add("name,name,name", "name")
	f.Add("  -name  ", "name")
	f.Add("\x00", "\x00")

	f.Fuzz(func(t *testing.T, sortString, allowedCSV string) {
		allowed := strings.Split(allowedCSV, ",")

		sort, err := ParseSortStringStrict(sortString, allowed)
		if err != nil {
			require.Nil(t, sort, "a rejected sort must not also be returned")
			return
		}

		allowSet := make(map[string]struct{}, len(allowed))
		for _, f := range allowed {
			allowSet[f] = struct{}{}
		}

		for _, entry := range sort {
			require.NotContains(t, entry.Key, "$",
				"an operator key reached the sort specification: %q", entry.Key)
			require.Contains(t, allowSet, entry.Key,
				"a field outside the allowlist reached the sort specification: %q", entry.Key)
			require.Contains(t, []any{1, -1}, entry.Value,
				"sort direction must be 1 or -1, got %v for %q", entry.Value, entry.Key)
		}
	})
}

// FuzzParseSortString_DropsOperatorKeys covers the lenient parser, which has no
// allowlist and therefore exactly one invariant left: an operator key never
// survives it. That is a weaker guarantee than Strict's on purpose — the point
// here is that the weaker one still holds for every input.
func FuzzParseSortString_DropsOperatorKeys(f *testing.F) {
	f.Add("name,-age")
	f.Add("$natural")
	f.Add("-$natural,name")
	f.Add("")
	f.Add(strings.Repeat("$", 32))

	f.Fuzz(func(t *testing.T, sortString string) {
		for _, entry := range ParseSortString(sortString) {
			require.False(t, corestrings.IsEmpty(entry.Key), "an empty sort key is not a field")
			require.NotEqual(t, byte('$'), entry.Key[0],
				"an operator key reached the sort specification: %q", entry.Key)
		}
	})
}
