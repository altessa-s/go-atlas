// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"

	redisstore "github.com/altessa-s/go-atlas/service/scheduler/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

// newDueIT builds a Storage over a live Redis with its own key prefix and its
// own RediSearch indexes, all dropped on cleanup. Unlike the ClaimRun fixture
// this one must call EnsureIndexes: DueTasks is served by FT.SEARCH, so without
// the index the query fails rather than merely running slowly.
func newDueIT(t *testing.T, opts ...redisstore.Option) *redisstore.Storage {
	t.Helper()

	ctx := t.Context()
	client := goredis.NewClient(&goredis.Options{Addr: redisAddr()})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Skipf("redis not reachable at %s: %v", redisAddr(), err)
	}

	prefix := "sched_due_it_" + itSuffix()
	s := redisstore.New(client, append([]redisstore.Option{redisstore.WithKeyPrefix(prefix)}, opts...)...)

	if err := s.EnsureIndexes(ctx); err != nil {
		_ = client.Close()
		t.Skipf("redis lacks the RedisJSON/RediSearch modules (need Redis Stack): %v", err)
	}

	t.Cleanup(func() {
		// t.Context() is canceled by the time cleanups run.
		cleanupCtx := context.WithoutCancel(ctx)
		_ = client.FTDropIndex(cleanupCtx, prefix+":idx:tasks").Err()
		_ = client.FTDropIndex(cleanupCtx, prefix+":idx:history").Err()
		if keys, _ := client.Keys(cleanupCtx, prefix+":*").Result(); len(keys) > 0 {
			_ = client.Del(cleanupCtx, keys...)
		}
		_ = client.Close()
	})

	return s
}

// collectDue drains DueTasks. It returns an error instead of failing the test
// so it is safe to call from require.Eventually's polling goroutine.
func collectDue(ctx context.Context, s *redisstore.Storage, now int64) ([]*scheduler.TaskState, error) {
	var states []*scheduler.TaskState
	for state, err := range s.DueTasks(ctx, now) {
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}

// TestIntegration_RedisDueTasks exercises the DueTasks predicate against a live
// Redis, where it is a RediSearch numeric-range query rather than a Go filter.
// The memory backend cannot catch a mistranslated query, and a wrong predicate
// here is silent from the scheduler's side: too narrow and tasks quietly stop
// firing, too wide and they fire early.
func TestIntegration_RedisDueTasks(t *testing.T) {
	t.Parallel()

	const now int64 = 1000

	s := newDueIT(t)
	ctx := t.Context()

	seed := []struct {
		id        string
		status    scheduler.TaskStatus
		nextRunAt int64
	}{
		{"a-due", scheduler.TaskStatusActive, now - 100},
		{"b-due", scheduler.TaskStatusActive, now - 1},
		{"c-exactly-now", scheduler.TaskStatusActive, now},
		{"d-future", scheduler.TaskStatusActive, now + 1},
		{"e-paused", scheduler.TaskStatusPaused, now - 1},
		{"f-disabled", scheduler.TaskStatusDisabled, now - 1},
		{"g-completed", scheduler.TaskStatusCompleted, now - 1},
		{"h-running", scheduler.TaskStatusRunning, now - 1},
	}
	for _, sd := range seed {
		require.NoError(t, s.UpsertTask(ctx, &scheduler.TaskState{
			TaskSummary: scheduler.TaskSummary{ID: sd.id, Status: sd.status, NextRunAt: sd.nextRunAt},
		}))
	}

	want := []string{"a-due", "b-due", "c-exactly-now"}

	// RediSearch indexes writes asynchronously, so the due set converges rather
	// than being correct on the first read.
	require.Eventually(t, func() bool {
		states, err := collectDue(ctx, s, now)
		return err == nil && len(states) == len(want)
	}, 5*time.Second, 50*time.Millisecond, "RediSearch never converged on the due set")

	states, err := collectDue(ctx, s, now)
	require.NoError(t, err)

	got := make([]string, 0, len(states))
	for _, st := range states {
		got = append(got, st.ID)
	}
	require.Equal(t, want, got, "boundary is inclusive: NextRunAt == now is due")

	// The summary fields live in an embedded struct, and the document→state
	// conversion used to drop them silently, which left every task with a zero
	// Status — neither active nor running, so invisible to both dispatch and
	// stale recovery. Assert the payload, not just the ID set.
	for _, st := range states {
		require.Equalf(t, scheduler.TaskStatusActive, st.Status, "task %q lost its status in conversion", st.ID)
		require.LessOrEqualf(t, st.NextRunAt, now, "task %q lost its NextRunAt in conversion", st.ID)
	}
}
