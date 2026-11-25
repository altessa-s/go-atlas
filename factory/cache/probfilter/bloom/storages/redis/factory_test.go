// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"bytes"
	"context"
	"iter"
	"log/slog"
	"slices"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter"
	"github.com/altessa-s/go-atlas/factory/cache/probfilter/bloom/storages/redis"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"

	bloomredis "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages/redis"
	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	probfilterfactory "github.com/altessa-s/go-atlas/factory/cache/probfilter"
	goredis "github.com/redis/go-redis/v9"
)

// redisBloomCfg is a Redis-backed Bloom config without rebuild on start
// (miniredis cannot run RedisBloom commands).
func redisBloomCfg() *probfilterconfig.Filter {
	return &probfilterconfig.Filter{
		Type: probfilterconfig.TypeBloom,
		Bloom: &probfilterconfig.BloomConfig{
			Storage:        &probfilterconfig.Storage{Type: probfilterconfig.StorageTypeRedis},
			ExpectedItems:  100,
			RebuildOnStart: new(false),
		},
	}
}

func defaults() *probfilterconfig.Defaults {
	d := probfilterconfig.NewDefaults()
	return &d
}

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

func TestBuilder_UsesClientNameAndPrefix(t *testing.T) {
	t.Parallel()
	client, mr := redistest.RedisClient(t)
	redistest.ServeTopology(mr, "master", "0")

	var mu sync.Mutex
	var reserves [][]string
	require.NoError(t, mr.Server().Register("BF.RESERVE", func(c *server.Peer, _ string, args []string) {
		mu.Lock()
		defer mu.Unlock()
		reserves = append(reserves, slices.Clone(args))
		c.WriteOK()
	}))

	sched := &testhelpers.MockTaskRegistrar{}
	cfg := redisBloomCfg()
	cfg.Bloom.Storage.Redis = &storageconfig.RedisConfig{KeysPrefix: "svc:"}
	cfg.Bloom.RebuildCron = new("@every 5m")
	loader := probfilter.NewDataLoader(func() iter.Seq[string] { return slices.Values([]string{"a"}) })

	f, err := probfilterfactory.NewFilter("users", cfg, defaults()).
		SkipEvictionPolicyCheck(true).
		UseBloomStorage(probfilterconfig.StorageTypeRedis, factory.Builder(client)).
		UseDataLoader(loader).
		UseScheduler(sched).
		Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.(interface{ Close(context.Context) error }).Close(context.WithoutCancel(t.Context())) })

	_, ok := sched.Task("probfilter-rebuild-users")
	require.True(t, ok, "a Redis filter is shared and rebuilt through the scheduler")

	// The staging reserve carries the configured prefix and filter name.
	// miniredis cannot materialize the module value, so the rebuild fails
	// later; only the reserve arguments matter here.
	_ = f.(probfilter.RebuildableFilter).Rebuild(t.Context(), loader)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, reserves)
	require.Contains(t, reserves[0][0], "{svc:users}", "the key carries the configured prefix and the filter name")
}

func TestBuilder_ClientRequired(t *testing.T) {
	t.Parallel()

	_, err := factory.Builder(nil)(t.Context(), probfilterfactory.BloomStorageSpec{})
	require.ErrorIs(t, err, factory.ErrClientRequired)

	storage, err := factory.Builder(redisClient(t))(t.Context(), probfilterfactory.BloomStorageSpec{
		Name: "users", SkipEvictionPolicyCheck: true,
	})
	require.NoError(t, err)
	require.IsType(t, &bloomredis.Storage{}, storage)
}

func redisClient(t *testing.T) *goredis.Client {
	t.Helper()
	client, _ := redistest.RedisClient(t)
	return client
}

// serveEvictionPolicy makes the miniredis behind client answer CONFIG GET
// maxmemory-policy with policy.
func serveEvictionPolicy(t *testing.T, policy string) *goredis.Client {
	t.Helper()
	client, mr := redistest.RedisClient(t)
	redistest.ServeTopology(mr, "master", "0")
	require.NoError(t, mr.Server().Register("CONFIG", func(c *server.Peer, _ string, _ []string) {
		c.WriteMapLen(1)
		c.WriteBulk("maxmemory-policy")
		c.WriteBulk(policy)
	}))
	return client
}

func TestBuilder_EvictionPolicyCheck(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T, client goredis.UniversalClient, logger *slog.Logger, skip bool) error {
		t.Helper()
		fb := probfilterfactory.NewFilter("users", redisBloomCfg(), defaults()).
			UseBloomStorage(probfilterconfig.StorageTypeRedis, factory.Builder(client)).
			SkipEvictionPolicyCheck(skip)
		if logger != nil {
			fb.UseLogger(logger)
		}
		_, err := fb.Build(t.Context())
		return err
	}

	t.Run("allkeys_fails_build", func(t *testing.T) {
		t.Parallel()
		err := build(t, serveEvictionPolicy(t, "allkeys-lru"), nil, false)
		require.ErrorIs(t, err, probfilter.ErrUnsafeEvictionPolicy)
	})

	t.Run("skip", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, build(t, serveEvictionPolicy(t, "allkeys-lru"), nil, true))
	})

	t.Run("volatile_passes", func(t *testing.T) {
		t.Parallel()
		logger, logs := newLogger()
		require.NoError(t, build(t, serveEvictionPolicy(t, "volatile-lru"), logger, false))
		require.NotContains(t, logs.String(), "not verified")
	})

	t.Run("unreadable_warns", func(t *testing.T) {
		t.Parallel()
		client, mr := redistest.RedisClient(t)
		redistest.ServeTopology(mr, "master", "0")
		require.NoError(t, mr.Server().Register("CONFIG", func(c *server.Peer, _ string, _ []string) {
			c.WriteError("NOPERM this user has no permissions to run the 'config' command")
		}))
		logger, logs := newLogger()
		require.NoError(t, build(t, client, logger, false))
		require.Contains(t, logs.String(), "maxmemory-policy not verified")
	})

	t.Run("manager_maps_skip_from_config", func(t *testing.T) {
		t.Parallel()
		cfg := &probfilterconfig.Config{
			Defaults:                defaults(),
			Filters:                 map[string]*probfilterconfig.Filter{"users": redisBloomCfg()},
			SkipEvictionPolicyCheck: true,
		}
		mgr, err := probfilterfactory.NewManager(cfg).
			UseBloomStorage(probfilterconfig.StorageTypeRedis, factory.Builder(serveEvictionPolicy(t, "allkeys-lru"))).
			Build(t.Context())
		require.NoError(t, err)
		require.NoError(t, mgr.Close())
	})
}
