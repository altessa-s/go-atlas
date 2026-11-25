// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package txscope_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/txscope"
)

func TestSavepointAncestors(t *testing.T) {
	t.Parallel()
	root := t.Context()
	outer, rollback := txscope.Begin(root)
	inner, release := txscope.Begin(outer)
	scope := txscope.Current(inner)
	outerScope := txscope.Current(outer)
	other, done := txscope.Begin(root)
	defer done(false)

	require.True(t, scope.OpenWithin(nil))
	require.False(t, scope.OpenWithin(txscope.Current(other)))
	require.False(t, scope.ReleasedWithin(nil))
	scope.Seal()
	require.False(t, scope.OpenWithin(outerScope))
	require.False(t, scope.ReleasedWithin(outerScope))
	release(true)
	require.False(t, scope.OpenWithin(outerScope))
	require.True(t, scope.ReleasedWithin(outerScope))
	require.False(t, scope.ReleasedWithin(nil))
	rollback(false)
	require.False(t, scope.ReleasedWithin(nil))
}
