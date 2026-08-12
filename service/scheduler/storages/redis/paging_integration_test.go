// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"
)

// TestIntegration_RedisReadsCrossSearchPages checks that a read whose match set
// spans more than one FT.SEARCH page returns every match exactly once.
//
// What this guards is the paging loop itself, not the ceiling it replaced: the
// old code asked for up to ten thousand documents in one shot, so a fixture
// this size would not have tripped it. Walking pages introduces its own failure
// modes — an off-by-one in the offset step drops a whole page, a wrong
// termination check stops after the first, and a stale one loops forever. All
// three are silent, which is exactly the property that made the original cap
// worth removing.
func TestIntegration_RedisReadsCrossSearchPages(t *testing.T) {
	t.Parallel()

	// Comfortably more than searchPageSize so the walk needs a second page and
	// the last page is partial.
	const total = 1100

	s := newDueIT(t)
	ctx := t.Context()

	for i := range total {
		require.NoError(t, s.UpsertTask(ctx, &scheduler.TaskState{
			TaskSummary: scheduler.TaskSummary{
				ID:        fmt.Sprintf("task-%04d", i),
				Status:    scheduler.TaskStatusActive,
				NextRunAt: 1,
			},
		}))
	}

	// RediSearch indexes asynchronously, so the match set converges.
	require.Eventually(t, func() bool {
		states, err := collectDue(ctx, s, 10)
		return err == nil && len(states) == total
	}, 30*time.Second, 200*time.Millisecond, "paged read never returned the full match set")

	states, err := collectDue(ctx, s, 10)
	require.NoError(t, err)
	require.Len(t, states, total)

	// A repeated offset would resend a page rather than lose one, which the
	// length check alone cannot distinguish from correct behavior.
	seen := make(map[string]struct{}, len(states))
	for _, st := range states {
		require.NotContainsf(t, seen, st.ID, "task %q returned more than once", st.ID)
		seen[st.ID] = struct{}{}
	}

	require.Equal(t, "task-0000", states[0].ID, "pages must stay in the requested sort order")
	require.Equal(t, fmt.Sprintf("task-%04d", total-1), states[total-1].ID)
}
