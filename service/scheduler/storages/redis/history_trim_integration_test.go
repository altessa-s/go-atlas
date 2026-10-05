// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"

	redisstore "github.com/altessa-s/go-atlas/service/scheduler/storages/redis"
)

// historyIDs drains History for a task. It returns an error rather than failing
// the test so it is safe to call from require.Eventually's polling goroutine.
func historyIDs(ctx context.Context, s *redisstore.Storage, taskID string) ([]string, error) {
	var ids []string
	for h, err := range s.History(ctx, taskID) {
		if err != nil {
			return nil, err
		}
		ids = append(ids, h.ID)
	}
	return ids, nil
}

// TestIntegration_RedisHistoryTrimEnforcesCap checks that the trim bounds a
// task's history at all.
//
// Scope, stated plainly: this does NOT verify which end gets dropped —
// trimming runs inside AddHistory while RediSearch may still be indexing, so
// each trim can act on a stale view. TestIntegration_RedisHistoryTrimIsCaseSensitive
// checks the kept entries on a fixture small enough to index synchronously.
func TestIntegration_RedisHistoryTrimEnforcesCap(t *testing.T) {
	t.Parallel()

	const (
		maxPerTask = 3
		added      = 10
		taskID     = "trimmed"
	)

	s := newDueIT(t, redisstore.WithMaxHistoryPerTask(maxPerTask))
	ctx := t.Context()

	for i := 1; i <= added; i++ {
		require.NoError(t, s.AddHistory(ctx, &scheduler.TaskHistory{
			ID:        fmt.Sprintf("h-%02d", i),
			TaskID:    taskID,
			StartedAt: int64(i),
			EndedAt:   int64(i),
			Success:   true,
		}))
	}

	// Trimming is best-effort and RediSearch indexes asynchronously, so the
	// history converges on the cap rather than hitting it on the last write.
	require.Eventually(t, func() bool {
		ids, err := historyIDs(ctx, s, taskID)
		return err == nil && len(ids) <= maxPerTask
	}, 30*time.Second, 200*time.Millisecond, "history never converged on the cap")

	ids, err := historyIDs(ctx, s, taskID)
	require.NoError(t, err)
	require.LessOrEqualf(t, len(ids), maxPerTask,
		"history must be bounded by the configured cap, got %d entries", len(ids))
	require.NotEmpty(t, ids, "trimming must not empty the history outright")
}

// TestIntegration_RedisHistoryTrimIsCaseSensitive checks that trimming one
// task's history leaves another task's alone when the two IDs differ only by
// case. The TAG query on taskId folds case, so the trim used to count both
// tasks' entries against the cap and delete the oldest of either.
func TestIntegration_RedisHistoryTrimIsCaseSensitive(t *testing.T) {
	t.Parallel()

	s := newDueIT(t, redisstore.WithMaxHistoryPerTask(2))
	ctx := t.Context()

	for i, h := range []struct {
		id, task string
	}{{"T1", "T"}, {"T2", "T"}, {"t1", "t"}, {"t2", "t"}, {"t3", "t"}} {
		require.NoError(t, s.AddHistory(ctx, &scheduler.TaskHistory{
			ID: h.id, TaskID: h.task, StartedAt: int64(i + 1), EndedAt: int64(i + 1), Success: true,
		}))
	}

	ids, err := historyIDs(ctx, s, "T")
	require.NoError(t, err)
	require.Equal(t, []string{"T2", "T1"}, ids, "trimming t must not touch T")

	ids, err = historyIDs(ctx, s, "t")
	require.NoError(t, err)
	require.Equal(t, []string{"t3", "t2"}, ids, "t keeps its newest entries up to the cap")
}
