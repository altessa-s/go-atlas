// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"context"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/scheduler"

	redisstore "github.com/altessa-s/go-atlas/service/scheduler/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

// redisAddr returns the Redis address, honoring REDIS_ADDR for CI.
func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "localhost:6379"
}

// newClaimIT connects to a live Redis and returns a Storage with a per-test key
// prefix (all keys dropped on cleanup). It skips when Redis is unreachable or
// lacks the RedisJSON module that the JSON-backed Storage requires.
func newClaimIT(t *testing.T) *redisstore.Storage {
	t.Helper()

	client := goredis.NewClient(&goredis.Options{Addr: redisAddr()})
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		t.Skipf("redis not reachable at %s: %v", redisAddr(), err)
	}

	// Capability probe: the Storage is built on RedisJSON (JSON.SET/JSON.GET).
	// A bare Redis without the module cannot run these tests.
	probe := "sched_claim_it_probe:" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := client.JSONSet(context.Background(), probe, "$", `{"ok":1}`).Err(); err != nil {
		_ = client.Del(context.Background(), probe)
		_ = client.Close()
		t.Skipf("redis lacks the RedisJSON module (need Redis Stack): %v", err)
	}
	_ = client.Del(context.Background(), probe)

	prefix := "sched_claim_it_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	t.Cleanup(func() {
		keys, _ := client.Keys(context.Background(), prefix+":*").Result()
		if len(keys) > 0 {
			_ = client.Del(context.Background(), keys...)
		}
		_ = client.Close()
	})

	return redisstore.New(client, redisstore.WithKeyPrefix(prefix))
}

// seedActive inserts an active task due at nextRunAt.
func seedActive(t *testing.T, s *redisstore.Storage, id string, nextRunAt int64) {
	t.Helper()
	require.NoError(t, s.UpsertTask(t.Context(), &scheduler.TaskState{
		TaskSummary: scheduler.TaskSummary{ID: id, Status: scheduler.TaskStatusActive, NextRunAt: nextRunAt},
	}))
}

func TestIntegration_RedisClaimRun_SingleWinnerThenLost(t *testing.T) {
	t.Parallel()
	s := newClaimIT(t)
	ctx := t.Context()
	seedActive(t, s, "a", 100)

	ok, err := s.ClaimRun(ctx, "a", 100, 1700000000, "run-1")
	require.NoError(t, err)
	require.True(t, ok, "first claim must win")

	got, err := s.GetTask(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, scheduler.TaskStatusRunning, got.Status)
	require.Equal(t, int64(1700000000), got.RunStartedAt)
	require.Equal(t, "run-1", got.LastRunID)

	ok, err = s.ClaimRun(ctx, "a", 100, 1700000001, "run-2")
	require.NoError(t, err)
	require.False(t, ok, "second claim must lose")
}

func TestIntegration_RedisClaimRun_FenceMismatch(t *testing.T) {
	t.Parallel()
	s := newClaimIT(t)
	seedActive(t, s, "a", 100)

	ok, err := s.ClaimRun(t.Context(), "a", 999, 1700000000, "run-1")
	require.NoError(t, err)
	require.False(t, ok, "a mismatched occurrence fence must not be claimable")
}

func TestIntegration_RedisClaimRun_ZeroFenceIgnoresNextRun(t *testing.T) {
	t.Parallel()
	s := newClaimIT(t)
	seedActive(t, s, "a", 100)

	ok, err := s.ClaimRun(t.Context(), "a", 0, 1700000000, "run-1")
	require.NoError(t, err)
	require.True(t, ok, "expectedNextRunAt==0 claims any active occurrence")
}

func TestIntegration_RedisClaimRun_NotActive(t *testing.T) {
	t.Parallel()
	s := newClaimIT(t)
	require.NoError(t, s.UpsertTask(t.Context(), &scheduler.TaskState{
		TaskSummary: scheduler.TaskSummary{ID: "a", Status: scheduler.TaskStatusPaused, NextRunAt: 100},
	}))

	ok, err := s.ClaimRun(t.Context(), "a", 100, 1700000000, "run-1")
	require.NoError(t, err)
	require.False(t, ok, "a paused task must not be claimable")
}

func TestIntegration_RedisClaimRun_MissingTask(t *testing.T) {
	t.Parallel()
	s := newClaimIT(t)

	ok, err := s.ClaimRun(t.Context(), "nope", 100, 1700000000, "run-1")
	require.NoError(t, err)
	require.False(t, ok)
}

// TestIntegration_RedisClaimRun_ExactlyOneConcurrentWinner is the core
// anti-double-execution invariant verified against Redis's real atomic EVAL:
// many schedulers racing on one occurrence yield exactly one winner.
func TestIntegration_RedisClaimRun_ExactlyOneConcurrentWinner(t *testing.T) {
	t.Parallel()
	s := newClaimIT(t)
	ctx := t.Context()
	seedActive(t, s, "a", 100)

	const racers = 32
	var wins atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range racers {
		wg.Go(func() {
			<-start
			ok, err := s.ClaimRun(ctx, "a", 100, 1700000000, "run")
			require.NoError(t, err)
			if ok {
				wins.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()

	require.Equal(t, int64(1), wins.Load(), "exactly one claim may win")
}
