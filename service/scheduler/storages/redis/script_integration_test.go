// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"

	goredis "github.com/redis/go-redis/v9"
)

// TestScriptsSurviveScriptFlush empties the server's script cache before every
// scripted operation, so each one is answered NOSCRIPT for its EVALSHA and must
// fall back to EVAL with unchanged results.
func TestScriptsSurviveScriptFlush(t *testing.T) {
	t.Parallel()
	store := newClaimIT(t)
	admin := goredis.NewClient(&goredis.Options{Addr: redisAddr()})
	t.Cleanup(func() { _ = admin.Close() })
	ctx := t.Context()
	flush := func() {
		t.Helper()
		require.NoError(t, admin.ScriptFlush(ctx).Err())
	}

	flush()
	created, err := store.CreateTask(ctx, &scheduler.TaskState{TaskSummary: scheduler.TaskSummary{
		ID: "flush", Status: scheduler.TaskStatusActive, Schedule: "@every 1m", NextRunAt: 300,
	}})
	require.NoError(t, err)
	require.True(t, created)

	flush()
	claimed, err := store.ClaimRun(ctx, "flush", scheduler.RunClaim{NextRunAt: 300, StartedAt: 100, RunID: "a/run", LeaseUntil: 400})
	require.NoError(t, err)
	require.True(t, claimed)

	flush()
	renewed, err := store.RenewRun(ctx, "flush", "a/run", 500)
	require.NoError(t, err)
	require.True(t, renewed)

	flush()
	finished, err := store.FinishRun(ctx, "flush", "a/run",
		scheduler.RunResult{StartedAt: 100, EndedAt: 110, NextRunAt: 360, Schedule: "@every 1m", Success: true})
	require.NoError(t, err)
	require.True(t, finished)

	got, err := store.GetTask(ctx, "flush")
	require.NoError(t, err)
	got.Description = "replaced"
	flush()
	replaced, err := store.ReplaceTaskIf(ctx, got, scheduler.FenceOf(got))
	require.NoError(t, err)
	require.True(t, replaced)

	got.Description = "upserted"
	flush()
	require.NoError(t, store.UpsertTask(ctx, got))

	got, err = store.GetTask(ctx, "flush")
	require.NoError(t, err)
	require.Equal(t, "upserted", got.Description)
	require.Equal(t, int64(6), got.Revision, "every scripted write ran exactly once")
	require.Equal(t, int64(360), got.NextRunAt)
	require.Zero(t, got.RunLeaseUntil)
}
