// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// White-box: chunkList and arena are internal allocator plumbing with no
// exported surface to test through.
package behavior

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChunkListAlloc(t *testing.T) {
	t.Parallel()

	var c chunkList[int]

	first := c.alloc(3, 4)
	require.Len(t, first, 3)
	require.Empty(t, c.full)

	// The remaining capacity (1) cannot hold 2 elements: cur is sealed into
	// full and the earlier slice keeps aliasing it.
	second := c.alloc(2, 4)
	require.Len(t, second, 2)
	require.Len(t, c.full, 1)
	require.Equal(t, 3, c.sealed)

	first[0] = 42
	require.Equal(t, 42, c.full[0][0], "alloc must alias chunk storage")

	// An oversized request opens a chunk sized to it.
	big := c.alloc(9, 4)
	require.Len(t, big, 9)
	require.Equal(t, 9, cap(c.cur))
}

func TestChunkListAllocExactWithoutChunkLen(t *testing.T) {
	t.Parallel()

	var c chunkList[int]
	s := c.alloc(2, 0)
	require.Equal(t, 2, cap(s), "zero chunkLen keeps the first allocation exact-sized")
}

func TestArenaResetClearsAndRewinds(t *testing.T) {
	t.Parallel()

	a := &arena{chunkLen: 8}
	fs := a.allocFields(2)
	fs[0].Name = "leftover"
	a.allocFields(16) // force a spill so reset also covers the full list

	a.reset()

	require.Empty(t, a.fields.full, "reset drops sealed chunks to the GC")
	require.Zero(t, a.fields.sealed)
	again := a.allocFields(2)
	require.Empty(t, again[0].Name, "reset must zero the recycled cur chunk")
}
