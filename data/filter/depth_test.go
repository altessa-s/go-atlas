// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
)

func TestDepthGuard(t *testing.T) {
	t.Parallel()

	t.Run("admits exactly max levels", func(t *testing.T) {
		t.Parallel()

		g := filter.NewDepthGuard(3)
		for i := range 3 {
			require.NoError(t, g.Enter(), "level %d should fit", i+1)
		}
		require.ErrorIs(t, g.Enter(), filter.ErrMaxDepthExceeded)
	})

	t.Run("leave frees a level", func(t *testing.T) {
		t.Parallel()

		g := filter.NewDepthGuard(1)
		require.NoError(t, g.Enter())
		require.ErrorIs(t, g.Enter(), filter.ErrMaxDepthExceeded)

		g.Leave()
		require.NoError(t, g.Enter(), "the level released by Leave should be reusable")
	})

	// A refused Enter must not consume a level, or an expression that
	// recovers from one rejection would find its budget short.
	t.Run("a refused enter records nothing", func(t *testing.T) {
		t.Parallel()

		g := filter.NewDepthGuard(1)
		require.NoError(t, g.Enter())
		for range 5 {
			require.ErrorIs(t, g.Enter(), filter.ErrMaxDepthExceeded)
		}

		g.Leave()
		require.NoError(t, g.Enter())
	})

	// This is what Reset is for: a walk that aborted mid-expression
	// leaves the guard part-consumed, and the next call must start fresh.
	t.Run("reset returns to zero depth", func(t *testing.T) {
		t.Parallel()

		g := filter.NewDepthGuard(2)
		require.NoError(t, g.Enter())
		require.NoError(t, g.Enter())
		require.ErrorIs(t, g.Enter(), filter.ErrMaxDepthExceeded)

		g.Reset()
		require.NoError(t, g.Enter())
		require.NoError(t, g.Enter())
		require.ErrorIs(t, g.Enter(), filter.ErrMaxDepthExceeded)
	})

	t.Run("the zero value refuses everything", func(t *testing.T) {
		t.Parallel()

		var g filter.DepthGuard
		require.ErrorIs(t, g.Enter(), filter.ErrMaxDepthExceeded)
	})
}
