// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	memory "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
)

// TestStorage_FullFilterKeepsMembers fills filters until inserts fail and
// checks that no failed insert evicted a previously added value: the
// no-false-negative invariant for added items.
func TestStorage_FullFilterKeepsMembers(t *testing.T) {
	t.Parallel()

	for _, capacity := range []uint{8, 64, 1024} {
		t.Run(fmt.Sprint(capacity), func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			s := memory.New(memory.WithCapacity(capacity))

			var added []string
			failures := 0
			for i := 0; failures < 100; i++ {
				v := fmt.Sprintf("v-%d", i)
				err := s.Add(ctx, v)
				if err != nil {
					require.ErrorIs(t, err, memory.ErrFilterFull)
					failures++
					continue
				}
				added = append(added, v)
			}

			for _, v := range added {
				ok, err := s.MightExist(ctx, v)
				require.NoError(t, err)
				require.True(t, ok, "member %q lost after failed inserts", v)
			}

			stats, err := s.Stats(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(len(added)), stats.ItemCount)
			require.GreaterOrEqual(t, float64(len(added)), 0.85*float64(max(capacity, 4)),
				"the filter should reach a high load factor before failing")
		})
	}
}

func TestStorage_DeleteThenReAdd(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s := memory.New(memory.WithCapacity(256))

	for i := range 200 {
		require.NoError(t, s.Add(ctx, fmt.Sprintf("v-%d", i)))
	}
	for i := range 100 {
		deleted, err := s.Delete(ctx, fmt.Sprintf("v-%d", i))
		require.NoError(t, err)
		require.True(t, deleted)
	}
	for i := 100; i < 200; i++ {
		ok, err := s.MightExist(ctx, fmt.Sprintf("v-%d", i))
		require.NoError(t, err)
		require.True(t, ok, "deleting other values must not remove v-%d", i)
	}
	stats, err := s.Stats(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(100), stats.ItemCount)
}
