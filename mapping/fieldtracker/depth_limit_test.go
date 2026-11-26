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

type privateBase struct{ V int }

type privateHolder struct{ privateBase }

type selected struct {
	Keep   int
	Hidden int `json:"-"`
}

type eqPtr struct{ v int }

func (e *eqPtr) Equal(o *eqPtr) bool { return e.v == o.v } // Panics on a nil receiver or argument.

// TestDepthLimitKeepsFieldSelection: collapsing a difference to the enclosing path must select fields as the walk does.
func TestDepthLimitKeepsFieldSelection(t *testing.T) {
	t.Parallel()

	t.Run("promoted fields of a private embedded struct", func(t *testing.T) {
		t.Parallel()
		type record struct{ In privateHolder }
		before, after := record{privateHolder{privateBase{1}}}, record{privateHolder{privateBase{2}}}
		require.NotEmpty(t, fieldtracker.GetChangedFields(before, after, fieldtracker.WithMaxDepth(1)))
		require.Empty(t, fieldtracker.GetChangedFields(before, before, fieldtracker.WithMaxDepth(1)))
	})

	t.Run("a field excluded by its tag", func(t *testing.T) {
		t.Parallel()
		type middle struct{ S selected }
		type record struct{ M middle }
		hidden := fieldtracker.GetChangedFields(record{M: middle{selected{1, 1}}}, record{M: middle{selected{1, 2}}}, fieldtracker.WithMaxDepth(1))
		require.Empty(t, hidden, "a change only in a json:\"-\" field is not a change")
		kept := fieldtracker.GetChangedFields(record{M: middle{selected{1, 1}}}, record{M: middle{selected{2, 1}}}, fieldtracker.WithMaxDepth(1))
		require.Equal(t, []string{"m.s"}, kept)
	})
}

// TestEqualMethodIsNeverCalledOnNilReferences: a user Equal that dereferences its receiver must not be reached through a nil pointer.
func TestEqualMethodIsNeverCalledOnNilReferences(t *testing.T) {
	t.Parallel()

	type middle struct{ Ref *eqPtr }
	type record struct{ M middle }
	require.NotPanics(t, func() {
		paths := fieldtracker.GetChangedFields(record{M: middle{}}, record{M: middle{Ref: &eqPtr{1}}}, fieldtracker.WithMaxDepth(1))
		require.Equal(t, []string{"m.ref"}, paths)
	})
}
