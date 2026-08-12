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

// TestIntegration_RedisHistoryTrimEnforcesCap checks that the rewritten trim
// still bounds a task's history, i.e. that the count-then-fetch-overflow logic
// computes an overflow at all.
//
// Scope, stated plainly: this does NOT verify which end gets dropped. The
// rewrite flipped the sort from descending-then-skip to ascending-then-take,
// and reversing it back leaves this test green — trimming runs inside
// AddHistory while RediSearch is still indexing, so each trim acts on a stale
// view and which specific entries survive is not deterministic from here.
// Pinning the direction needs a fixture that lets the index settle and then
// triggers exactly one trim against a known-complete set; until that exists,
// treat the drop-oldest behavior as unverified.
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
