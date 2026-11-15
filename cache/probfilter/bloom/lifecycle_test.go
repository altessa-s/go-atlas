// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bloom_test

import (
	"bytes"
	"context"
	"errors"
	"iter"
	"log/slog"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter"
	"github.com/altessa-s/go-atlas/cache/probfilter/bloom"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"

	bloommemory "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages/memory"
	bloomredis "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages/redis"
	schedtask "github.com/altessa-s/go-horai/task"
)

// streamCounter streams one value and counts how often a rebuild streams it.
type streamCounter struct {
	streams atomic.Int32
	fail    bool
}

func (l *streamCounter) StreamValues(context.Context) iter.Seq2[string, error] {
	l.streams.Add(1)
	if l.fail {
		return func(yield func(string, error) bool) { yield("", errLoad) }
	}
	return func(yield func(string, error) bool) { yield("x", nil) }
}

func (l *streamCounter) Count(context.Context) (int64, error) { return 1, nil }

func newMemoryFilter(t *testing.T, opts ...bloom.Option) *bloom.Filter {
	t.Helper()
	f := bloom.New(bloommemory.New(bloommemory.WithExpectedItems(100)), opts...)
	t.Cleanup(func() { _ = f.Close(context.Background()) })
	return f
}

func TestStartRebuild_WithoutLoaderIsInert(t *testing.T) {
	t.Parallel()
	sched := &testhelpers.MockTaskRegistrar{}
	f := newMemoryFilter(t, bloom.WithRebuildOnStart(), bloom.WithRebuildSchedule("@every 1s"), bloom.WithScheduler(sched))

	require.NoError(t, f.StartRebuild(t.Context()))
	require.True(t, f.LastRebuild().IsZero())
	require.Zero(t, sched.Count())
}

func TestStartRebuild_OnStartPopulatesFilter(t *testing.T) {
	t.Parallel()
	loader := &streamCounter{}
	f := newMemoryFilter(t, bloom.WithRebuildLoader(loader), bloom.WithRebuildOnStart())

	require.NoError(t, f.StartRebuild(t.Context()))
	require.Equal(t, int32(1), loader.streams.Load())
	ok, err := f.MightExist(t.Context(), "x")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestStartRebuild_OnStartFailure(t *testing.T) {
	t.Parallel()
	f := newMemoryFilter(t, bloom.WithRebuildLoader(&streamCounter{fail: true}), bloom.WithRebuildOnStart())

	require.ErrorIs(t, f.StartRebuild(t.Context()), errLoad)
}

func TestStartRebuild_StartsOnce(t *testing.T) {
	t.Parallel()
	f := newMemoryFilter(t, bloom.WithRebuildLoader(&streamCounter{}), bloom.WithRebuildSchedule("@yearly"))

	require.NoError(t, f.StartRebuild(t.Context()))
	require.ErrorIs(t, f.StartRebuild(t.Context()), bloom.ErrRebuildAlreadyStarted)
}

func TestStartRebuild_InvalidSchedule(t *testing.T) {
	t.Parallel()
	f := newMemoryFilter(t, bloom.WithRebuildLoader(&streamCounter{}), bloom.WithRebuildSchedule("not a cron"))

	require.ErrorIs(t, f.StartRebuild(t.Context()), bloom.ErrInvalidRebuildSchedule)
}

// A process-local filter is rebuilt by a local cron, never by the scheduler,
// and the cron stops once the filter is closed. The bubble's fake clock runs
// the "@every 1s" schedule without waiting in real time.
func TestStartRebuild_LocalCronStopsOnClose(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		sched := &testhelpers.MockTaskRegistrar{}
		loader := &streamCounter{}
		f := bloom.New(bloommemory.New(), bloom.WithRebuildLoader(loader), bloom.WithRebuildSchedule("@every 1s"),
			bloom.WithScheduler(sched))

		require.NoError(t, f.StartRebuild(t.Context()))
		time.Sleep(1500 * time.Millisecond)
		synctest.Wait()
		require.GreaterOrEqual(t, loader.streams.Load(), int32(1), "local cron rebuild")
		require.Zero(t, sched.Count())

		require.NoError(t, f.Close(t.Context()))
		after := loader.streams.Load()
		time.Sleep(5 * time.Second)
		synctest.Wait()
		require.Equal(t, after, loader.streams.Load(), "a closed filter must not be rebuilt")
	})
}

// A shared filter registers its periodic rebuild with the scheduler.
func TestStartRebuild_SharedFilterRegistersTask(t *testing.T) {
	t.Parallel()
	client, _ := redistest.RedisClient(t)
	sched := &testhelpers.MockTaskRegistrar{}
	f := bloom.New(bloomredis.New(client, "users"),
		bloom.WithName("users"),
		bloom.WithRebuildLoader(&streamCounter{}),
		bloom.WithRebuildSchedule("@every 5m"),
		bloom.WithScheduler(sched))

	require.NoError(t, f.StartRebuild(t.Context()))
	task, ok := sched.Task(bloom.RebuildTaskPrefix + "users")
	require.True(t, ok)
	require.Equal(t, "@every 5m", task.Schedule)
	require.Equal(t, schedtask.PriorityNormal, task.Priority)
	require.Error(t, task.Func(t.Context()), "rebuild errors reach the scheduler")

	require.NoError(t, f.Close(t.Context()))
	require.NoError(t, task.Func(t.Context()), "a closed filter's task is a no-op")
}

func TestStartRebuild_SharedFilterWithoutScheduler(t *testing.T) {
	t.Parallel()
	client, _ := redistest.RedisClient(t)
	var logs bytes.Buffer
	loader := &streamCounter{}
	f := bloom.New(bloomredis.New(client, "users"),
		bloom.WithRebuildLoader(loader),
		bloom.WithRebuildSchedule("@every 5m"),
		bloom.WithLogger(slog.New(slog.NewTextHandler(&logs, nil))))
	t.Cleanup(func() { _ = f.Close(context.Background()) })

	require.NoError(t, f.StartRebuild(t.Context()))
	require.Contains(t, logs.String(), "rebuild schedule ignored, no scheduler injected for a shared filter")
	require.Zero(t, loader.streams.Load(), "no rebuild runs without a scheduler")
}

func TestStartRebuild_RegisterFailure(t *testing.T) {
	t.Parallel()
	client, _ := redistest.RedisClient(t)
	errRegister := errors.New("register failed")
	f := bloom.New(bloomredis.New(client, "users"),
		bloom.WithRebuildLoader(&streamCounter{}),
		bloom.WithRebuildSchedule("@every 5m"),
		bloom.WithScheduler(&testhelpers.MockTaskRegistrar{Err: errRegister}))
	t.Cleanup(func() { _ = f.Close(context.Background()) })

	require.ErrorIs(t, f.StartRebuild(t.Context()), errRegister)
}

var _ probfilter.DataLoader = (*streamCounter)(nil)
