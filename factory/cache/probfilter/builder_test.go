// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"bytes"
	"context"
	"errors"
	"iter"
	"log/slog"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter"
	"github.com/altessa-s/go-atlas/cache/probfilter/bloom"
	"github.com/altessa-s/go-atlas/cache/probfilter/probfiltertest"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	bloomstorages "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages"
	bloommemory "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages/memory"
	cuckoostorages "github.com/altessa-s/go-atlas/cache/probfilter/cuckoo/storages"
	cuckoomemory "github.com/altessa-s/go-atlas/cache/probfilter/cuckoo/storages/memory"
	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	factory "github.com/altessa-s/go-atlas/factory/cache/probfilter"
	schedtask "github.com/altessa-s/go-horai/task"
)

var errLoad = errors.New("load failed")

func bloomCfg() *probfilterconfig.Filter {
	return &probfilterconfig.Filter{
		Type:  probfilterconfig.TypeBloom,
		Bloom: &probfilterconfig.BloomConfig{ExpectedItems: 100},
	}
}

func defaults() *probfilterconfig.Defaults {
	d := probfilterconfig.NewDefaults()
	return &d
}

func loaderOf(values ...string) probfilter.DataLoader {
	return probfilter.NewDataLoader(func() iter.Seq[string] { return slices.Values(values) })
}

func failingLoader() probfilter.DataLoader {
	return probfilter.DataLoaderFunc(func(context.Context) iter.Seq2[string, error] {
		return func(yield func(string, error) bool) { yield("", errLoad) }
	})
}

// countingLoader counts how often a rebuild streams it.
type countingLoader struct {
	streams atomic.Int32
}

func (l *countingLoader) StreamValues(context.Context) iter.Seq2[string, error] {
	l.streams.Add(1)
	return func(yield func(string, error) bool) { yield("x", nil) }
}

func (l *countingLoader) Count(context.Context) (int64, error) { return 1, nil }

// logBuffer is a concurrency-safe log sink for asserting warnings.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func newLogger() (*slog.Logger, *logBuffer) {
	buf := &logBuffer{}
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})), buf
}

func mightExist(t *testing.T, f probfilter.Filter, v string) bool {
	t.Helper()
	ok, err := f.MightExist(t.Context(), v)
	require.NoError(t, err)
	return ok
}

func TestFilterBuilder_NoLoader_RebuildSettingsInert(t *testing.T) {
	t.Parallel()
	sched := &testhelpers.MockTaskRegistrar{}

	f, err := factory.NewFilter("users", bloomCfg(), defaults()).UseScheduler(sched).Build(t.Context())
	require.NoError(t, err)
	require.Zero(t, sched.Count())
	require.True(t, f.(probfilter.RebuildableFilter).LastRebuild().IsZero())
}

func TestFilterBuilder_RebuildOnStart_WithoutScheduler(t *testing.T) {
	t.Parallel()
	logger, logs := newLogger()

	f, err := factory.NewFilter("users", bloomCfg(), defaults()).
		UseLogger(logger).
		UseDataLoader(loaderOf("a", "b")).
		Build(t.Context())
	require.NoError(t, err)
	require.True(t, mightExist(t, f, "a"))
	require.True(t, mightExist(t, f, "b"))
	require.False(t, f.(probfilter.RebuildableFilter).LastRebuild().IsZero())
	require.NotContains(t, logs.String(), "rebuild schedule ignored", "an in-memory filter needs no scheduler for its cron")
}

func TestFilterBuilder_RebuildOnStartDisabled(t *testing.T) {
	t.Parallel()
	cfg := bloomCfg()
	cfg.Bloom.RebuildOnStart = new(false)
	cfg.Bloom.RebuildCron = new("")

	f, err := factory.NewFilter("users", cfg, defaults()).UseDataLoader(loaderOf("a")).Build(t.Context())
	require.NoError(t, err)
	require.False(t, mightExist(t, f, "a"))
}

func TestFilterBuilder_RebuildOnStart_Failure(t *testing.T) {
	t.Parallel()
	_, err := factory.NewFilter("users", bloomCfg(), defaults()).UseDataLoader(failingLoader()).Build(t.Context())
	require.ErrorIs(t, err, errLoad)
}

// redisBloomCfg is a Redis-backed Bloom config without rebuild on start.
func redisBloomCfg() *probfilterconfig.Filter {
	cfg := bloomCfg()
	cfg.Bloom.Storage = &probfilterconfig.Storage{Type: probfilterconfig.StorageTypeRedis}
	cfg.Bloom.RebuildOnStart = new(false)
	return cfg
}

func TestFilterBuilder_SharedFilterRegistersSchedulerTask(t *testing.T) {
	t.Parallel()
	sched := &testhelpers.MockTaskRegistrar{}
	cfg := redisBloomCfg()
	cfg.Bloom.RebuildCron = new("@every 5m")

	f, err := factory.NewFilter("users", cfg, defaults()).
		UseBloomStorage(probfilterconfig.StorageTypeRedis, sharedBloom(false)).
		UseDataLoader(loaderOf("a")).
		UseScheduler(sched).
		Build(t.Context())
	require.NoError(t, err)

	task, ok := sched.Task("probfilter-rebuild-users")
	require.True(t, ok)
	require.Equal(t, "@every 5m", task.Schedule)
	require.False(t, task.RunOnStart, "the initial rebuild runs inside Build, not via the scheduler")

	// Once the filter is closed the task is a no-op.
	require.NoError(t, f.(interface{ Close(context.Context) error }).Close(t.Context()))
	require.NoError(t, task.Func(t.Context()))
}

func TestFilterBuilder_SharedFilterWithoutSchedulerWarns(t *testing.T) {
	t.Parallel()
	logger, logs := newLogger()

	_, err := factory.NewFilter("users", redisBloomCfg(), defaults()).
		UseLogger(logger).
		UseBloomStorage(probfilterconfig.StorageTypeRedis, sharedBloom(false)).
		UseDataLoader(loaderOf("a")).
		Build(t.Context())
	require.NoError(t, err)
	require.Contains(t, logs.String(), "rebuild schedule ignored")
}

func TestFilterBuilder_MemoryFilterRebuildsOnStartEvenWithScheduler(t *testing.T) {
	t.Parallel()
	sched := &testhelpers.MockTaskRegistrar{}

	f, err := factory.NewFilter("users", bloomCfg(), defaults()).
		UseDataLoader(loaderOf("a")).
		UseScheduler(sched).
		Build(t.Context())
	require.NoError(t, err)
	require.True(t, mightExist(t, f, "a"), "rebuildOnStart populates every new filter inside Build")
	require.Zero(t, sched.Count(), "an in-memory filter is rebuilt by a process-local cron, not the shared scheduler")
}

// TestFilterBuilder_MemoryFilterLocalCron runs the process-local rebuild cron
// and checks that it stops rebuilding once the filter is closed.
func TestFilterBuilder_MemoryFilterLocalCron(t *testing.T) {
	t.Parallel()
	cfg := bloomCfg()
	cfg.Bloom.RebuildCron = new("@every 1s")
	loader := &countingLoader{}

	f, err := factory.NewFilter("users", cfg, defaults()).UseDataLoader(loader).Build(t.Context())
	require.NoError(t, err)
	require.Equal(t, int32(1), loader.streams.Load(), "initial rebuild")

	testhelpers.WaitFor(t, 5*time.Second, func() bool { return loader.streams.Load() >= 2 }, "local cron rebuild")

	require.NoError(t, f.(interface{ Close(context.Context) error }).Close(t.Context()))
	after := loader.streams.Load()
	time.Sleep(2500 * time.Millisecond)
	require.Equal(t, after, loader.streams.Load(), "a closed filter must not be rebuilt")
}

func TestFilterBuilder_InvalidCronFailsBuild(t *testing.T) {
	t.Parallel()
	cfg := bloomCfg()
	cfg.Bloom.RebuildCron = new("not a cron")

	_, err := factory.NewFilter("users", cfg, defaults()).UseDataLoader(loaderOf("a")).Build(t.Context())
	require.ErrorIs(t, err, bloom.ErrInvalidRebuildSchedule)
}

func TestFilterBuilder_EmptyCronWithScheduler_RebuildsSynchronously(t *testing.T) {
	t.Parallel()
	sched := &testhelpers.MockTaskRegistrar{}
	cfg := bloomCfg()
	cfg.Bloom.RebuildCron = new("")

	f, err := factory.NewFilter("users", cfg, defaults()).
		UseDataLoader(loaderOf("a")).
		UseScheduler(sched).
		Build(t.Context())
	require.NoError(t, err)
	require.Zero(t, sched.Count())
	require.True(t, mightExist(t, f, "a"))
}

func TestFilterBuilder_RegisterFailure(t *testing.T) {
	t.Parallel()
	errRegister := errors.New("register failed")
	sched := &testhelpers.MockTaskRegistrar{Err: errRegister}

	loader := &countingLoader{}
	_, err := factory.NewFilter("users", redisBloomCfg(), defaults()).
		UseBloomStorage(probfilterconfig.StorageTypeRedis, sharedBloom(false)).
		UseDataLoader(loader).
		UseScheduler(sched).
		Build(t.Context())
	require.ErrorIs(t, err, errRegister)

	// The filter was closed on failure, so the recorded task is a no-op.
	task, ok := sched.Task("probfilter-rebuild-users")
	require.True(t, ok)
	require.NoError(t, task.Func(t.Context()))
	require.Zero(t, loader.streams.Load(), "a closed filter must not rebuild")
}

func TestFilterBuilder_CuckooUnsupportedCapacityMultiplierWarn(t *testing.T) {
	t.Parallel()
	logger, logs := newLogger()
	cfg := &probfilterconfig.Filter{
		Type: probfilterconfig.TypeCuckoo,
		Cuckoo: &probfilterconfig.CuckooConfig{
			Capacity:           100,
			CapacityMultiplier: new(3.0),
		},
	}

	f, err := factory.NewFilter("sessions", cfg, defaults()).UseLogger(logger).Build(t.Context())
	require.NoError(t, err)
	_, rebuildable := f.(probfilter.RebuildableFilter)
	require.True(t, rebuildable, "Cuckoo filters are rebuildable")

	out := logs.String()
	require.Contains(t, out, "capacityMultiplier ignored")
}

func TestManagerBuilder_WiresLoadersSchedulerAndCollector(t *testing.T) {
	t.Parallel()
	sched := &testhelpers.MockTaskRegistrar{}
	tc := metricstest.NewTestCollector()
	cfg := &probfilterconfig.Config{
		Defaults: defaults(),
		Filters:  map[string]*probfilterconfig.Filter{"users": bloomCfg()},
	}

	mgr, err := factory.NewManager(cfg).
		UseDataLoader("users", loaderOf("a")).
		UseScheduler(sched).
		UseMeter(tc).
		Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })
	require.Zero(t, sched.Count(), "in-memory filters use a process-local cron")

	f := mgr.MustGet("users")
	require.True(t, mightExist(t, f, "a"))
	require.Equal(t, 1.0, metricstest.GetCounterValue(t, tc, "test_probfilter_lookups_total",
		"filter_name", "users", "result", "positive"))
}

func TestManagerBuilder_LaterFailureDeactivatesEarlierTasks(t *testing.T) {
	t.Parallel()
	sched := &testhelpers.MockTaskRegistrar{}
	broken := bloomCfg()
	broken.Bloom.RebuildCron = new("not a cron") // fails Build after the earlier filter registered its task

	cfg := &probfilterconfig.Config{
		Defaults: defaults(),
		Filters: map[string]*probfilterconfig.Filter{
			"a-users":  redisBloomCfg(),
			"b-broken": broken,
		},
	}

	loader := &countingLoader{}
	_, err := factory.NewManager(cfg).
		UseBloomStorage(probfilterconfig.StorageTypeRedis, sharedBloom(false)).
		UseDataLoader("a-users", loader).
		UseDataLoader("b-broken", loaderOf("x")).
		UseScheduler(sched).
		Build(t.Context())
	require.Error(t, err)

	task, ok := sched.Task("probfilter-rebuild-a-users")
	require.True(t, ok, "the earlier filter registered its task before the failure")
	require.NoError(t, task.Func(t.Context()))
	require.Zero(t, loader.streams.Load(), "the closed filter's task must not rebuild")
}

// blockingLoader returns at once on its first stream and blocks later streams
// until release is closed.
type blockingLoader struct {
	streams atomic.Int32
	release chan struct{}
}

func (l *blockingLoader) StreamValues(context.Context) iter.Seq2[string, error] {
	if l.streams.Add(1) > 1 {
		<-l.release
	}
	return func(yield func(string, error) bool) { yield("x", nil) }
}

func (l *blockingLoader) Count(context.Context) (int64, error) { return 1, nil }

// TestFilterBuilder_LocalCronSkipsTicksWhileRebuilding blocks a scheduled
// rebuild across several ticks: the missed ticks must be skipped, not queued.
func TestFilterBuilder_LocalCronSkipsTicksWhileRebuilding(t *testing.T) {
	t.Parallel()
	cfg := bloomCfg()
	cfg.Bloom.RebuildCron = new("@every 1s")
	loader := &blockingLoader{release: make(chan struct{})}

	f, err := factory.NewFilter("users", cfg, defaults()).UseDataLoader(loader).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.(interface{ Close(context.Context) error }).Close(context.Background()) })

	testhelpers.WaitFor(t, 5*time.Second, func() bool { return loader.streams.Load() >= 2 }, "scheduled rebuild started")
	time.Sleep(3500 * time.Millisecond) // several ticks while the rebuild is blocked
	blocked := loader.streams.Load()
	close(loader.release)
	// A queued backlog would run right after the release; skipped ticks
	// leave at most the next regular tick.
	time.Sleep(300 * time.Millisecond)

	require.Equal(t, int32(2), blocked, "ticks during a running rebuild must be skipped")
	require.LessOrEqual(t, loader.streams.Load(), int32(3), "skipped ticks must not run later as a backlog")
}

// TestFilterBuilder_CloseStopsLocalCronImmediately builds filters whose local
// cron would next fire in a year and closes them: their cron goroutines must
// stop right away, not at the next tick.
//
// Not parallel: it compares the process-wide goroutine count, which parallel
// tests would disturb (they start only after the sequential tests finished).
func TestFilterBuilder_CloseStopsLocalCronImmediately(t *testing.T) {
	cfg := bloomCfg()
	cfg.Bloom.RebuildCron = new("@yearly")
	baseline := runtime.NumGoroutine()

	const n = 20
	filters := make([]probfilter.Filter, 0, n)
	for range n {
		f, err := factory.NewFilter("users", cfg, defaults()).UseDataLoader(loaderOf("a")).Build(t.Context())
		require.NoError(t, err)
		filters = append(filters, f)
	}
	require.GreaterOrEqual(t, runtime.NumGoroutine(), baseline+n, "each filter runs a local cron")

	for _, f := range filters {
		require.NoError(t, f.(interface{ Close(context.Context) error }).Close(t.Context()))
	}
	testhelpers.WaitFor(t, 5*time.Second, func() bool { return runtime.NumGoroutine() <= baseline+2 },
		"local crons must stop when their filters close")
}

func TestFilterBuilder_RebuildOnStart_PeerRebuildingFailsBuild(t *testing.T) {
	t.Parallel()
	cfg := redisBloomCfg()
	cfg.Bloom.RebuildOnStart = new(true)

	_, err := factory.NewFilter("users", cfg, defaults()).
		UseBloomStorage(probfilterconfig.StorageTypeRedis, sharedBloom(true)).
		UseDataLoader(loaderOf("a")).
		Build(t.Context())
	require.ErrorIs(t, err, probfilter.ErrRebuildInProgress, "by default Build never returns an unpopulated filter")
}

func TestFilterBuilder_RebuildOnStart_TolerateRebuildInProgress(t *testing.T) {
	t.Parallel()
	cfg := redisBloomCfg()
	cfg.Bloom.RebuildOnStart = new(true)
	logger, logs := newLogger()

	f, err := factory.NewFilter("users", cfg, defaults()).
		UseLogger(logger).
		UseBloomStorage(probfilterconfig.StorageTypeRedis, sharedBloom(true)).
		UseDataLoader(loaderOf("a")).
		TolerateRebuildInProgress().
		Build(t.Context())
	require.NoError(t, err)
	require.True(t, f.(probfilter.RebuildableFilter).LastRebuild().IsZero())
	require.Contains(t, logs.String(), "initial rebuild skipped")
	require.NoError(t, f.(interface{ Close(context.Context) error }).Close(t.Context()))
}

func TestFilterBuilder_RebuildOnStart_TolerateKeepsOtherErrors(t *testing.T) {
	t.Parallel()
	_, err := factory.NewFilter("users", bloomCfg(), defaults()).
		UseDataLoader(failingLoader()).
		TolerateRebuildInProgress().
		Build(t.Context())
	require.ErrorIs(t, err, errLoad)
}

type startupCtxKey struct{}

// The initial rebuild runs on the caller's context, so a startup deadline or
// cancellation reaches the data loader.
func TestFilterBuilder_RebuildOnStartUsesCallerContext(t *testing.T) {
	t.Parallel()
	cfg := bloomCfg()
	cfg.Bloom.RebuildOnStart = new(true)
	cfg.Bloom.RebuildCron = new("")

	var got any
	loader := probfilter.DataLoaderFunc(func(ctx context.Context) iter.Seq2[string, error] {
		got = ctx.Value(startupCtxKey{})
		return func(yield func(string, error) bool) { yield("a", nil) }
	})

	ctx := context.WithValue(t.Context(), startupCtxKey{}, "startup")
	f, err := factory.NewFilter("users", cfg, defaults()).UseDataLoader(loader).Build(ctx)
	require.NoError(t, err)
	require.Equal(t, "startup", got)
	require.True(t, mightExist(t, f, "a"))
}

// The scheduled task of a shared filter reports loader failures to the
// scheduler, with normal priority.
func TestFilterBuilder_SharedTaskReportsRebuildErrors(t *testing.T) {
	t.Parallel()
	sched := &testhelpers.MockTaskRegistrar{}
	cfg := redisBloomCfg()
	cfg.Bloom.RebuildCron = new("@every 5m")

	f, err := factory.NewFilter("users", cfg, defaults()).
		UseBloomStorage(probfilterconfig.StorageTypeRedis, sharedBloom(false)).
		UseDataLoader(failingLoader()).
		UseScheduler(sched).
		Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.(interface{ Close(context.Context) error }).Close(context.Background()) })

	task, ok := sched.Task("probfilter-rebuild-users")
	require.True(t, ok)
	require.Equal(t, schedtask.PriorityNormal, task.Priority)
	require.ErrorIs(t, task.Func(t.Context()), errLoad)
}

// The scheduled task of a shared filter is a no-op while another process
// holds the rebuild lease: that rebuild publishes a fresh snapshot.
func TestFilterBuilder_SharedTaskSkipsRebuildInProgress(t *testing.T) {
	t.Parallel()
	sched := &testhelpers.MockTaskRegistrar{}
	cfg := redisBloomCfg()
	cfg.Bloom.RebuildCron = new("@every 5m")

	f, err := factory.NewFilter("users", cfg, defaults()).
		UseBloomStorage(probfilterconfig.StorageTypeRedis, sharedBloom(true)).
		UseDataLoader(loaderOf("a")).
		UseScheduler(sched).
		Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.(interface{ Close(context.Context) error }).Close(context.Background()) })

	task, ok := sched.Task("probfilter-rebuild-users")
	require.True(t, ok)
	require.NoError(t, task.Func(t.Context()))
}

// failAfterFirstLoader succeeds on its first stream and fails afterwards.
type failAfterFirstLoader struct {
	streams atomic.Int32
}

func (l *failAfterFirstLoader) StreamValues(context.Context) iter.Seq2[string, error] {
	if l.streams.Add(1) > 1 {
		return func(yield func(string, error) bool) { yield("", errLoad) }
	}
	return func(yield func(string, error) bool) { yield("x", nil) }
}

func (l *failAfterFirstLoader) Count(context.Context) (int64, error) { return 1, nil }

// A failed tick of the process-local cron is logged and the cron keeps running.
func TestFilterBuilder_LocalCronLogsFailedRebuild(t *testing.T) {
	t.Parallel()
	cfg := bloomCfg()
	cfg.Bloom.RebuildCron = new("@every 1s")
	logger, logs := newLogger()
	loader := &failAfterFirstLoader{}

	f, err := factory.NewFilter("users", cfg, defaults()).UseLogger(logger).UseDataLoader(loader).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.(interface{ Close(context.Context) error }).Close(context.Background()) })

	testhelpers.WaitFor(t, 5*time.Second, func() bool { return loader.streams.Load() >= 3 }, "cron keeps running after a failure")
	require.Contains(t, logs.String(), "scheduled rebuild failed")
	require.True(t, mightExist(t, f, "x"), "a failed rebuild keeps the previous contents")
}

var errBuild = errors.New("storage build failed")

func TestFilterBuilder_UnregisteredRedisStorageFails(t *testing.T) {
	t.Parallel()

	_, err := factory.NewFilter("users", redisBloomCfg(), defaults()).Build(t.Context())
	require.Error(t, err, "bloom redis not registered")

	cfg := &probfilterconfig.Filter{
		Type: probfilterconfig.TypeCuckoo,
		Cuckoo: &probfilterconfig.CuckooConfig{
			Storage:  &probfilterconfig.Storage{Type: probfilterconfig.StorageTypeRedis},
			Capacity: 100,
		},
	}
	_, err = factory.NewFilter("sessions", cfg, defaults()).Build(t.Context())
	require.Error(t, err, "cuckoo redis not registered")

	unknown := bloomCfg()
	unknown.Bloom.Storage = &probfilterconfig.Storage{Type: "nats"}
	_, err = factory.NewFilter("users", unknown, defaults()).Build(t.Context())
	require.Error(t, err, "unsupported type")
}

func TestFilterBuilder_UseBloomStorage(t *testing.T) {
	t.Parallel()

	_, err := factory.NewFilter("users", redisBloomCfg(), defaults()).
		UseBloomStorage(probfilterconfig.StorageTypeRedis, nil).
		Build(t.Context())
	require.Error(t, err)

	failing := func(context.Context, factory.BloomStorageSpec) (bloomstorages.Storage, error) { return nil, errBuild }
	_, err = factory.NewFilter("users", redisBloomCfg(), defaults()).
		UseBloomStorage(probfilterconfig.StorageTypeRedis, failing).
		Build(t.Context())
	require.ErrorIs(t, err, errBuild)

	cfg := redisBloomCfg()
	cfg.Bloom.Storage.Redis = nil
	cfg.Bloom.FalsePositiveRate = new(0.05)
	var got factory.BloomStorageSpec
	f, err := factory.NewFilter("users", cfg, defaults()).
		UseBloomStorage(probfilterconfig.StorageTypeRedis, failing). // replaced by the later registration
		UseBloomStorage(probfilterconfig.StorageTypeRedis, func(_ context.Context, spec factory.BloomStorageSpec) (bloomstorages.Storage, error) {
			got = spec
			return bloommemory.New(), nil
		}).
		SkipEvictionPolicyCheck(true).
		Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, f)
	require.Equal(t, "users", got.Name)
	require.Same(t, cfg.Bloom.Storage, got.Storage)
	require.EqualValues(t, 100, got.ExpectedItems)
	require.InDelta(t, 0.05, got.FalsePositiveRate, 1e-9)
	require.True(t, got.SkipEvictionPolicyCheck)
	require.NotNil(t, got.Logger)
}

func TestFilterBuilder_UseCuckooStorage(t *testing.T) {
	t.Parallel()
	cfg := &probfilterconfig.Filter{
		Type: probfilterconfig.TypeCuckoo,
		Cuckoo: &probfilterconfig.CuckooConfig{
			Storage:            &probfilterconfig.Storage{Type: probfilterconfig.StorageTypeRedis},
			Capacity:           100,
			CapacityMultiplier: new(2.5),
		},
	}

	_, err := factory.NewFilter("sessions", cfg, defaults()).
		UseCuckooStorage(probfilterconfig.StorageTypeRedis, nil).
		Build(t.Context())
	require.Error(t, err)

	failing := func(context.Context, factory.CuckooStorageSpec) (cuckoostorages.Storage, error) { return nil, errBuild }
	_, err = factory.NewFilter("sessions", cfg, defaults()).
		UseCuckooStorage(probfilterconfig.StorageTypeRedis, failing).
		Build(t.Context())
	require.ErrorIs(t, err, errBuild)

	var got factory.CuckooStorageSpec
	_, err = factory.NewFilter("sessions", cfg, defaults()).
		UseCuckooStorage(probfilterconfig.StorageTypeRedis, func(_ context.Context, spec factory.CuckooStorageSpec) (cuckoostorages.Storage, error) {
			got = spec
			return cuckoomemory.New(), nil
		}).
		Build(t.Context())
	require.NoError(t, err)
	require.Equal(t, "sessions", got.Name)
	require.EqualValues(t, 100, got.Capacity)
	require.InDelta(t, 2.5, got.CapacityMultiplier, 1e-9)
}

func TestManagerBuilder_UseStorage(t *testing.T) {
	t.Parallel()
	cfg := &probfilterconfig.Config{
		Defaults: defaults(),
		Filters:  map[string]*probfilterconfig.Filter{"users": redisBloomCfg()},
	}

	_, err := factory.NewManager(cfg).Build(t.Context())
	require.Error(t, err, "redis not registered")

	_, err = factory.NewManager(cfg).UseBloomStorage(probfilterconfig.StorageTypeRedis, nil).Build(t.Context())
	require.Error(t, err)

	_, err = factory.NewManager(cfg).UseCuckooStorage(probfilterconfig.StorageTypeRedis, nil).Build(t.Context())
	require.Error(t, err)

	var built atomic.Int32
	mgr, err := factory.NewManager(cfg).
		UseBloomStorage(probfilterconfig.StorageTypeRedis, func(context.Context, factory.BloomStorageSpec) (bloomstorages.Storage, error) {
			built.Add(1)
			return bloommemory.New(), nil
		}).
		UseCuckooStorage(probfilterconfig.StorageTypeRedis, func(context.Context, factory.CuckooStorageSpec) (cuckoostorages.Storage, error) {
			return cuckoomemory.New(), nil
		}).
		Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })
	require.Equal(t, int32(1), built.Load())
}

// sharedBloom returns a storage function building a shared in-memory storage,
// for registration as the redis type.
func sharedBloom(peerRebuilding bool) factory.BloomStorageFunc {
	return func(context.Context, factory.BloomStorageSpec) (bloomstorages.Storage, error) {
		return probfiltertest.NewSharedBloom(peerRebuilding), nil
	}
}
