// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldtracker_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/mapping/fieldtracker"
)

// TestDifferenceBeyondMaxDepthIsReportedAtTheEnclosingPath: a change below the depth limit must not read as "unchanged", or an update
// mask built from the result drops the update.
func TestDifferenceBeyondMaxDepthIsReportedAtTheEnclosingPath(t *testing.T) {
	t.Parallel()

	type c struct{ C int }
	type b struct{ B c }
	type a struct{ A b }
	before, after := a{b{c{1}}}, a{b{c{2}}}

	require.Equal(t, []string{"a.b"}, fieldtracker.GetChangedFields(before, after, fieldtracker.WithMaxDepth(1)))
	require.Equal(t, []string{"a.b.c"}, fieldtracker.GetChangedFields(before, after, fieldtracker.WithMaxDepth(3)))
	require.Empty(t, fieldtracker.GetChangedFields(before, before, fieldtracker.WithMaxDepth(1)))
}

// TestEqualValuesAreNotChanged: opaque value types that are Equal but not DeepEqual (a time in another location) are unchanged.
func TestEqualValuesAreNotChanged(t *testing.T) {
	t.Parallel()

	type record struct{ At time.Time }
	utc := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	other := utc.In(time.FixedZone("x", 3600))
	require.True(t, utc.Equal(other))

	require.Empty(t, fieldtracker.GetChangedFields(record{utc}, record{other}))
	require.Equal(t, []string{"at"}, fieldtracker.GetChangedFields(record{utc}, record{utc.Add(time.Second)}))
}
