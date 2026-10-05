// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"context"
	"fmt"
	"net"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"

	redisstore "github.com/altessa-s/go-atlas/service/scheduler/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

// legacyTaskJSON is a task document as written while next_run_at was omitted
// when zero: an active task, due, with no next_run_at at all.
func legacyTaskJSON(id string, status scheduler.TaskStatus) string {
	return fmt.Sprintf(`{"id":%q,"status":%d,"priority":0,"schedule":"@every 1m","failures":0,"created_at":1,"updated_at":1,"revision":3}`,
		id, status)
}

// newBackfillIT returns a client and a per-test key prefix over a live Redis
// Stack, with every key and index under the prefix dropped on cleanup. No
// Storage is built and no index is created, so a test can seed documents
// before the index exists.
func newBackfillIT(t *testing.T) (*goredis.Client, string) {
	t.Helper()
	client := goredis.NewClient(&goredis.Options{Addr: redisAddr()})
	if err := client.Ping(t.Context()).Err(); err != nil {
		_ = client.Close()
		t.Skipf("redis not reachable at %s: %v", redisAddr(), err)
	}
	prefix := "sched_backfill_it_" + itSuffix()
	if err := client.JSONSet(t.Context(), prefix+":probe", "$", `{"ok":1}`).Err(); err != nil {
		_ = client.Close()
		t.Skipf("redis lacks the RedisJSON module (need Redis Stack): %v", err)
	}
	t.Cleanup(func() {
		ctx := context.WithoutCancel(t.Context())
		_ = client.FTDropIndex(ctx, prefix+":idx:tasks").Err()
		_ = client.FTDropIndex(ctx, prefix+":idx:history").Err()
		if keys, _ := client.Keys(ctx, prefix+":*").Result(); len(keys) > 0 {
			_ = client.Del(ctx, keys...)
		}
		_ = client.Close()
	})
	return client, prefix
}

func dueIDs(t *testing.T, s *redisstore.Storage, now int64) []string {
	t.Helper()
	states, err := collectDue(t.Context(), s, now)
	require.NoError(t, err)
	ids := make([]string, 0, len(states))
	for _, st := range states {
		ids = append(ids, st.ID)
	}
	return ids
}

// TestIntegration_RedisEnsureIndexesBackfillsNextRunAt pins the migration of
// documents stored without next_run_at: RediSearch does not index them, so
// DueTasks misses them until EnsureIndexes stores the zero. The backfill keeps
// the revision (absent and zero are the same state) and is idempotent.
func TestIntegration_RedisEnsureIndexesBackfillsNextRunAt(t *testing.T) {
	t.Parallel()
	client, prefix := newBackfillIT(t)
	ctx := t.Context()
	s := redisstore.New(client, redisstore.WithKeyPrefix(prefix))
	require.NoError(t, s.EnsureIndexes(ctx))

	require.NoError(t, client.JSONSet(ctx, prefix+":task:legacy", "$", legacyTaskJSON("legacy", scheduler.TaskStatusActive)).Err())
	require.NoError(t, client.JSONSet(ctx, prefix+":task:paused", "$", legacyTaskJSON("paused", scheduler.TaskStatusPaused)).Err())
	require.Empty(t, dueIDs(t, s, 200), "a document without next_run_at is invisible to the due query")

	require.NoError(t, s.EnsureIndexes(ctx))
	require.Equal(t, []string{"legacy"}, dueIDs(t, s, 200))
	raw, err := client.JSONGet(ctx, prefix+":task:paused", "$.next_run_at").Result()
	require.NoError(t, err)
	require.Equal(t, "[0]", raw, "every document lacking the field is migrated, whatever its status")

	got, err := s.GetTask(ctx, "legacy")
	require.NoError(t, err)
	require.Equal(t, int64(3), got.Revision, "the backfill must not change the revision")
	require.Zero(t, got.NextRunAt)
	claimed, err := s.ClaimRun(ctx, "legacy", scheduler.RunClaim{NextRunAt: 0, StartedAt: 100, RunID: "owner/run"})
	require.NoError(t, err)
	require.True(t, claimed, "a backfilled task must be claimable")

	require.NoError(t, s.EnsureIndexes(ctx), "the backfill is idempotent")
	got, err = s.GetTask(ctx, "paused")
	require.NoError(t, err)
	require.Equal(t, int64(3), got.Revision)
}

// TestIntegration_RedisEnsureIndexesBackfillsBeforeIndexExists covers
// documents written before the task index is created: FT.CREATE indexes them
// in the background, and the backfill must wait for that rather than search a
// partial index.
func TestIntegration_RedisEnsureIndexesBackfillsBeforeIndexExists(t *testing.T) {
	t.Parallel()
	client, prefix := newBackfillIT(t)
	ctx := t.Context()

	const n = 3000
	pipe := client.Pipeline()
	want := make([]string, 0, n)
	for i := range n {
		id := fmt.Sprintf("t%05d", i)
		want = append(want, id)
		pipe.JSONSet(ctx, prefix+":task:"+id, "$", legacyTaskJSON(id, scheduler.TaskStatusActive))
	}
	_, err := pipe.Exec(ctx)
	require.NoError(t, err)

	s := redisstore.New(client, redisstore.WithKeyPrefix(prefix))
	require.NoError(t, s.EnsureIndexes(ctx))
	require.Equal(t, want, dueIDs(t, s, 200), "every pre-existing document must be backfilled")
}

// rewriteFirstBackfillPage is a client hook standing in for a concurrent
// writer: right after the first backfill search returns, before any document
// of that page is migrated, it calls rewrite with the page's keys.
type rewriteFirstBackfillPage struct {
	rewrite func(ctx context.Context, keys []string) error
	done    atomic.Bool
}

func (h *rewriteFirstBackfillPage) DialHook(next goredis.DialHook) goredis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) { return next(ctx, network, addr) }
}

func (h *rewriteFirstBackfillPage) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}

func (h *rewriteFirstBackfillPage) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		err := next(ctx, cmd)
		search, ok := cmd.(*goredis.FTSearchCmd)
		if err != nil || !ok || !slices.Contains(cmd.Args(), any("-@nextRunAt:[-inf +inf]")) || !h.done.CompareAndSwap(false, true) {
			return err
		}
		keys := make([]string, 0, len(search.Val().Docs))
		for _, doc := range search.Val().Docs {
			keys = append(keys, doc.ID)
		}
		return h.rewrite(ctx, keys)
	}
}

// backfillAgainstWriter seeds n legacy documents, runs EnsureIndexes through a
// client whose first backfill page is first passed to rewrite, and returns the
// storage and the number of documents still lacking an indexed next_run_at.
func backfillAgainstWriter(t *testing.T, n int, rewrite func(ctx context.Context, writer *goredis.Client, keys []string) error) (*redisstore.Storage, int) {
	t.Helper()
	writer, prefix := newBackfillIT(t)
	ctx := t.Context()

	pipe := writer.Pipeline()
	for i := range n {
		id := fmt.Sprintf("t%05d", i)
		pipe.JSONSet(ctx, prefix+":task:"+id, "$", legacyTaskJSON(id, scheduler.TaskStatusActive))
	}
	_, err := pipe.Exec(ctx)
	require.NoError(t, err)

	hook := &rewriteFirstBackfillPage{rewrite: func(ctx context.Context, keys []string) error { return rewrite(ctx, writer, keys) }}
	client := goredis.NewClient(&goredis.Options{Addr: redisAddr()})
	t.Cleanup(func() { _ = client.Close() })
	client.AddHook(hook)

	s := redisstore.New(client, redisstore.WithKeyPrefix(prefix))
	require.NoError(t, s.EnsureIndexes(ctx))
	require.True(t, hook.done.Load(), "the concurrent rewrite never ran")

	left, err := writer.FTSearchWithArgs(ctx, prefix+":idx:tasks", "-@nextRunAt:[-inf +inf]",
		&goredis.FTSearchOptions{NoContent: true, LimitOffset: 0, Limit: 0}).Result()
	require.NoError(t, err)
	return s, left.Total
}

// TestIntegration_RedisBackfillToleratesConcurrentWriters pins that matches
// leaving the result set between pages — rewritten by another instance — do
// not make the backfill skip the documents behind them, as offset paging over
// a shrinking result set would.
func TestIntegration_RedisBackfillToleratesConcurrentWriters(t *testing.T) {
	t.Parallel()
	const n = 2500
	s, left := backfillAgainstWriter(t, n, func(ctx context.Context, writer *goredis.Client, keys []string) error {
		for _, key := range keys {
			if err := writer.JSONSet(ctx, key, "$.next_run_at", 7).Err(); err != nil {
				return err
			}
		}
		return nil
	})
	require.Zero(t, left, "every legacy document must be migrated")
	require.Len(t, dueIDs(t, s, 200), n)
}

// TestIntegration_RedisBackfillToleratesUnindexableRewrite covers a concurrent
// writer storing a non-numeric next_run_at: RediSearch drops that document from
// the index, and the backfill must still migrate every document behind it.
func TestIntegration_RedisBackfillToleratesUnindexableRewrite(t *testing.T) {
	t.Parallel()
	const n = 1001
	s, left := backfillAgainstWriter(t, n, func(ctx context.Context, writer *goredis.Client, keys []string) error {
		return writer.JSONSet(ctx, keys[0], "$.next_run_at", `"bad"`).Err()
	})
	require.Zero(t, left, "every legacy document must be migrated")
	require.Len(t, dueIDs(t, s, 200), n-1, "all but the rewritten document are due")
}

// TestIntegration_RedisBackfillSkipsUnmigratable checks that documents whose
// next_run_at holds a non-numeric value are left alone, and that a null
// next_run_at is migrated like an absent one.
func TestIntegration_RedisBackfillSkipsUnmigratable(t *testing.T) {
	t.Parallel()
	client, prefix := newBackfillIT(t)
	ctx := t.Context()

	const n = 1500
	pipe := client.Pipeline()
	for i := range n {
		id := fmt.Sprintf("t%05d", i)
		doc := legacyTaskJSON(id, scheduler.TaskStatusActive)
		switch i % 3 {
		case 1:
			doc = doc[:len(doc)-1] + `,"next_run_at":"bad"}`
		case 2:
			doc = doc[:len(doc)-1] + `,"next_run_at":null}`
		}
		pipe.JSONSet(ctx, prefix+":task:"+id, "$", doc)
	}
	_, err := pipe.Exec(ctx)
	require.NoError(t, err)

	s := redisstore.New(client, redisstore.WithKeyPrefix(prefix))
	require.NoError(t, s.EnsureIndexes(ctx))

	left, err := client.FTSearchWithArgs(ctx, prefix+":idx:tasks", "-@nextRunAt:[-inf +inf]",
		&goredis.FTSearchOptions{NoContent: true, LimitOffset: 0, Limit: 0}).Result()
	require.NoError(t, err)
	// RediSearch rejects a document whose NUMERIC field is not a number, so the
	// non-numeric ones are not even matched; the script leaves them alone.
	require.Zero(t, left.Total)
	raw, err := client.JSONGet(ctx, prefix+":task:t00001", "$.next_run_at").Result()
	require.NoError(t, err)
	require.Equal(t, `["bad"]`, raw)
	raw, err = client.JSONGet(ctx, prefix+":task:t00002", "$.next_run_at").Result()
	require.NoError(t, err)
	require.Equal(t, "[0]", raw)
}
