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

	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom"
	"github.com/altessa-s/go-atlas/data/probfilter/factory"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	bloomredis "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

var errLoad = errors.New("load failed")

func bloomCfg() *config.ProbabilisticFilterConfig {
	return &config.ProbabilisticFilterConfig{
		Type:  config.ProbabilisticFilterTypeBloom,
		Bloom: &config.ProbabilisticFilterBloomConfig{ExpectedItems: 100},
	}
}

func defaults() *config.ProbabilisticFilterDefaults {
	d := config.DefaultProbabilisticFilterDefaults()
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

	f, err := factory.NewFilter("users", bloomCfg(), defaults()).UseScheduler(sched).Build()
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
		Build()
	require.NoError(t, err)
	require.True(t, mightExist(t, f, "a"))
	require.True(t, mightExist(t, f, "b"))
	require.False(t, f.(probfilter.RebuildableFilter).LastRebuild().IsZero())
	require.NotContains(t, logs.String(), "rebuildCron ignored", "an in-memory filter needs no scheduler for its cron")
}

func TestFilterBuilder_RebuildOnStartDisabled(t *testing.T) {
	t.Parallel()
	cfg := bloomCfg()
	cfg.Bloom.RebuildOnStart = new(false)
	cfg.Bloom.RebuildCron = new("")

	f, err := factory.NewFilter("users", cfg, defaults()).UseDataLoader(loaderOf("a")).Build()
	require.NoError(t, err)
	require.False(t, mightExist(t, f, "a"))
}

func TestFilterBuilder_RebuildOnStart_Failure(t *testing.T) {
	t.Parallel()
	_, err := factory.NewFilter("users", bloomCfg(), defaults()).UseDataLoader(failingLoader()).Build()
	require.ErrorIs(t, err, errLoad)
}

// redisBloomCfg is a Redis-backed Bloom config without rebuild on start
// (miniredis cannot run RedisBloom commands).
func redisBloomCfg() *config.ProbabilisticFilterConfig {
	cfg := bloomCfg()
	storage := config.ProbabilisticFilterStorageTypeRedis
	cfg.Bloom.Storage = &storage
	cfg.Bloom.RebuildOnStart = new(false)
	return cfg
}

func TestFilterBuilder_SharedFilterRegistersSchedulerTask(t *testing.T) {
	t.Parallel()
	client, _ := testhelpers.RedisClient(t)
	sched := &testhelpers.MockTaskRegistrar{}
	cfg := redisBloomCfg()
	cfg.Bloom.RebuildCron = new("@every 5m")

	f, err := factory.NewFilter("users", cfg, defaults()).
		UseRedisClient(client).
		UseDataLoader(loaderOf("a")).
		UseScheduler(sched).
		Build()
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
	client, _ := testhelpers.RedisClient(t)
	logger, logs := newLogger()

	_, err := factory.NewFilter("users", redisBloomCfg(), defaults()).
		UseLogger(logger).
		UseRedisClient(client).
		UseDataLoader(loaderOf("a")).
		Build()
	require.NoError(t, err)
	require.Contains(t, logs.String(), "rebuildCron ignored")
}

func TestFilterBuilder_MemoryFilterRebuildsOnStartEvenWithScheduler(t *testing.T) {
	t.Parallel()
	sched := &testhelpers.MockTaskRegistrar{}

	f, err := factory.NewFilter("users", bloomCfg(), defaults()).
		UseDataLoader(loaderOf("a")).
		UseScheduler(sched).
		Build()
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

	f, err := factory.NewFilter("users", cfg, defaults()).UseDataLoader(loader).Build()
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

	_, err := factory.NewFilter("users", cfg, defaults()).UseDataLoader(loaderOf("a")).Build()
	require.ErrorContains(t, err, "invalid rebuildCron")
}

func TestFilterBuilder_EmptyCronWithScheduler_RebuildsSynchronously(t *testing.T) {
	t.Parallel()
	sched := &testhelpers.MockTaskRegistrar{}
	cfg := bloomCfg()
	cfg.Bloom.RebuildCron = new("")

	f, err := factory.NewFilter("users", cfg, defaults()).
		UseDataLoader(loaderOf("a")).
		UseScheduler(sched).
		Build()
	require.NoError(t, err)
	require.Zero(t, sched.Count())
	require.True(t, mightExist(t, f, "a"))
}

func TestFilterBuilder_RegisterFailure(t *testing.T) {
	t.Parallel()
	client, _ := testhelpers.RedisClient(t)
	errRegister := errors.New("register failed")
	sched := &testhelpers.MockTaskRegistrar{Err: errRegister}

	loader := &countingLoader{}
	_, err := factory.NewFilter("users", redisBloomCfg(), defaults()).
		UseRedisClient(client).
		UseDataLoader(loader).
		UseScheduler(sched).
		Build()
	require.ErrorIs(t, err, errRegister)

	// The filter was closed on failure, so the recorded task is a no-op.
	task, ok := sched.Task("probfilter-rebuild-users")
	require.True(t, ok)
	require.NoError(t, task.Func(t.Context()))
	require.Zero(t, loader.streams.Load(), "a closed filter must not rebuild")
}

func TestFilterBuilder_CuckooDeprecatedSettingsWarn(t *testing.T) {
	t.Parallel()
	logger, logs := newLogger()
	cfg := &config.ProbabilisticFilterConfig{
		Type: config.ProbabilisticFilterTypeCuckoo,
		Cuckoo: &config.ProbabilisticFilterCuckooConfig{
			Capacity:           100,
			FingerprintSize:    new(16),
			CapacityMultiplier: new(3.0),
			MaxCapacity:        new(int64(5000)),
		},
	}

	f, err := factory.NewFilter("sessions", cfg, defaults()).UseLogger(logger).Build()
	require.NoError(t, err)
	_, rebuildable := f.(probfilter.RebuildableFilter)
	require.True(t, rebuildable, "Cuckoo filters are rebuildable")

	out := logs.String()
	require.Contains(t, out, "deprecated fingerprintSize ignored")
	require.Contains(t, out, "deprecated maxCapacity ignored")
	require.Contains(t, out, "capacityMultiplier ignored")
}

func TestFilterBuilder_CuckooRedisExpansion(t *testing.T) {
	t.Parallel()
	client, mr := testhelpers.RedisClient(t)

	var mu sync.Mutex
	var reserves [][]string
	require.NoError(t, mr.Server().Register("CF.RESERVE", func(c *server.Peer, _ string, args []string) {
		mu.Lock()
		defer mu.Unlock()
		reserves = append(reserves, slices.Clone(args))
		c.WriteOK()
	}))
	require.NoError(t, mr.Server().Register("CF.INSERT", func(c *server.Peer, _ string, _ []string) { c.WriteInt(1) }))

	storage := config.ProbabilisticFilterStorageTypeRedis
	cfg := &config.ProbabilisticFilterConfig{
		Type: config.ProbabilisticFilterTypeCuckoo,
		Cuckoo: &config.ProbabilisticFilterCuckooConfig{
			Storage:            &storage,
			Capacity:           100,
			CapacityMultiplier: new(2.5),
		},
	}
	f, err := factory.NewFilter("sessions", cfg, defaults()).UseRedisClient(client).Build()
	require.NoError(t, err)

	// The staging reserve carries the configured expansion. miniredis cannot
	// materialize the module value, so the final RENAME fails; only the
	// reserve arguments matter here.
	_ = f.(probfilter.RebuildableFilter).Rebuild(t.Context(), loaderOf("a"))

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, reserves)
	require.Equal(t, []string{"100", "EXPANSION", "3"}, reserves[0][1:], "ceil(2.5) = 3")
}

func TestManagerBuilder_WiresLoadersSchedulerAndCollector(t *testing.T) {
	t.Parallel()
	sched := &testhelpers.MockTaskRegistrar{}
	tc := testhelpers.NewTestCollector()
	cfg := &config.ProbabilisticFilter{
		Defaults: defaults(),
		Filters:  map[string]*config.ProbabilisticFilterConfig{"users": bloomCfg()},
	}

	mgr, err := factory.NewManager(cfg).
		UseDataLoader("users", loaderOf("a")).
		UseScheduler(sched).
		UseCollector(tc).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })
	require.Zero(t, sched.Count(), "in-memory filters use a process-local cron")

	f := mgr.MustGet("users")
	require.True(t, mightExist(t, f, "a"))
	require.Equal(t, 1.0, testhelpers.GetCounterValue(t, tc, "test_probfilter_lookups_total",
		"filter_name", "users", "result", "positive"))
}

func TestManagerBuilder_LaterFailureDeactivatesEarlierTasks(t *testing.T) {
	t.Parallel()
	client, _ := testhelpers.RedisClient(t)
	sched := &testhelpers.MockTaskRegistrar{}
	broken := bloomCfg()
	broken.Bloom.RebuildCron = new("not a cron") // fails Build after the earlier filter registered its task

	cfg := &config.ProbabilisticFilter{
		Defaults: defaults(),
		Filters: map[string]*config.ProbabilisticFilterConfig{
			"a-users":  redisBloomCfg(),
			"b-broken": broken,
		},
	}

	loader := &countingLoader{}
	_, err := factory.NewManager(cfg).
		UseRedisClient(client).
		UseDataLoader("a-users", loader).
		UseDataLoader("b-broken", loaderOf("x")).
		UseScheduler(sched).
		Build()
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

	f, err := factory.NewFilter("users", cfg, defaults()).UseDataLoader(loader).Build()
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
		f, err := factory.NewFilter("users", cfg, defaults()).UseDataLoader(loaderOf("a")).Build()
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

// leaseHoldingLoader signals when a rebuild starts streaming it and blocks the
// stream until release is closed, keeping the rebuild lease held meanwhile.
type leaseHoldingLoader struct {
	started chan struct{}
	release chan struct{}
}

func (l *leaseHoldingLoader) StreamValues(context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		close(l.started)
		<-l.release
		yield("peer", nil)
	}
}

func (l *leaseHoldingLoader) Count(context.Context) (int64, error) { return -1, nil }

// holdRebuildLease starts a rebuild of the shared "users" filter by a peer
// process that holds the rebuild lease until the test ends.
func holdRebuildLease(t *testing.T, client goredis.UniversalClient) {
	t.Helper()
	peer := bloom.New(bloomredis.New(client, "users"))
	loader := &leaseHoldingLoader{started: make(chan struct{}), release: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = peer.Rebuild(context.Background(), loader) // miniredis cannot stage; the error is irrelevant
	}()
	<-loader.started
	t.Cleanup(func() {
		close(loader.release)
		<-done
	})
}

func TestFilterBuilder_RebuildOnStart_PeerRebuildingFailsBuild(t *testing.T) {
	t.Parallel()
	client, _ := testhelpers.RedisClient(t)
	holdRebuildLease(t, client)
	cfg := redisBloomCfg()
	cfg.Bloom.RebuildOnStart = new(true)

	_, err := factory.NewFilter("users", cfg, defaults()).
		UseRedisClient(client).
		UseDataLoader(loaderOf("a")).
		Build()
	require.ErrorIs(t, err, probfilter.ErrRebuildInProgress, "by default Build never returns an unpopulated filter")
}

func TestFilterBuilder_RebuildOnStart_TolerateRebuildInProgress(t *testing.T) {
	t.Parallel()
	client, _ := testhelpers.RedisClient(t)
	holdRebuildLease(t, client)
	cfg := redisBloomCfg()
	cfg.Bloom.RebuildOnStart = new(true)
	logger, logs := newLogger()

	f, err := factory.NewFilter("users", cfg, defaults()).
		UseLogger(logger).
		UseRedisClient(client).
		UseDataLoader(loaderOf("a")).
		TolerateRebuildInProgress().
		Build()
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
		Build()
	require.ErrorIs(t, err, errLoad)
}
