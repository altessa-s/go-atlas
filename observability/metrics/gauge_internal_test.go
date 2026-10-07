// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSeriesKey(t *testing.T) {
	t.Parallel()

	// Equivalent label sets share a key regardless of map construction order.
	a := Labels{"method": "GET", "code": "200"}
	b := Labels{"code": "200", "method": "GET"}
	require.Equal(t, seriesKey(a), seriesKey(b))

	// Separator-like characters in names or values must not cause collisions.
	distinct := []Labels{
		{"a": "1|b=2"},
		{"a": "1", "b": "2"},
		{"a": "1:b", "": ""},
		{"a1": ":b"},
		{"a": "11:b"},
		{"a": ""},
		{"": "a"},
	}
	seen := make(map[string]Labels, len(distinct))
	for _, l := range distinct {
		key := seriesKey(l)
		prev, dup := seen[key]
		require.False(t, dup, "label sets %v and %v share key %q", prev, l, key)
		seen[key] = l
	}
}
