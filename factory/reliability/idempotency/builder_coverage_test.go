// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/altessa-s/go-atlas/factory/reliability/idempotency"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"

	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	memorystorage "github.com/altessa-s/go-atlas/reliability/idempotency/storages/memory"
	schedtask "github.com/altessa-s/go-horai/task"
)

var errBuild = errors.New("build failed")

type taskRecorder struct {
	mu    sync.Mutex
	tasks []schedtask.Config
}

func (r *taskRecorder) Register(_ context.Context, cfg schedtask.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks = append(r.tasks, cfg)
	return nil
}

func (r *taskRecorder) schedules() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.tasks))
	for _, task := range r.tasks {
		out = append(out, task.Schedule)
	}
	return out
}

func storageCfg(st *idempotencyconfig.StorageConfig) *idempotencyconfig.Config {
	return &idempotencyconfig.Config{TTL: time.Hour, MaxLockDuration: time.Minute, Storage: st}
}

func TestBuild_MemoryStorageDeduplicates(t *testing.T) {
	t.Parallel()
	sched := &taskRecorder{}
	tc := metricstest.NewTestCollector()
	cfg := storageCfg(&idempotencyconfig.StorageConfig{
		Type:   idempotencyconfig.StorageTypeMemory,
		Memory: &idempotencyconfig.MemoryStorageConfig{CleanupSchedule: "@every 1m"},
	})

	keeper, err := factory.New(cfg).
		UseLogger(slog.New(slog.DiscardHandler)).
		UseScheduler(sched).
		UseMeter(tc).
		Build(t.Context())
	require.NoError(t, err)

	ok, held, err := keeper.AttemptLock(t.Context(), "k")
	require.NoError(t, err)
	require.True(t, ok)
	ok, _, err = keeper.AttemptLock(t.Context(), "k")
	require.NoError(t, err)
	require.False(t, ok, "a held key must not be locked twice")
	require.NoError(t, keeper.Complete(t.Context(), "k", "done", held))

	require.Contains(t, sched.schedules(), "@every 1m", "the memory cleanup task must be registered")
	require.InDelta(t, 1, metricstest.GetCounterValue(t, tc, "test_idempotency_locks_acquired_total"), 0)
}

func TestBuild_MemoryCleanupRegistrationFailure(t *testing.T) {
	t.Parallel()
	rejected := errors.New("cleanup registration rejected")
	scheduler := &testhelpers.MockTaskRegistrar{Err: rejected}
	cfg := storageCfg(&idempotencyconfig.StorageConfig{
		Type:   idempotencyconfig.StorageTypeMemory,
		Memory: &idempotencyconfig.MemoryStorageConfig{CleanupSchedule: "@every 1m"},
	})

	keeper, err := factory.New(cfg).UseScheduler(scheduler).Build(t.Context())

	require.Equal(t, 1, scheduler.Count())
	require.ErrorIs(t, err, rejected)
	require.Nil(t, keeper)
}

func TestBuild_ConfigErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *idempotencyconfig.Config
		wantErr error
		wantMsg string
	}{
		{name: "nil config", cfg: nil, wantErr: corefactory.ErrConfigRequired},
		{name: "nil storage", cfg: storageCfg(nil), wantErr: corefactory.ErrConfigRequired},
		{name: "unsupported type", cfg: storageCfg(&idempotencyconfig.StorageConfig{Type: "etcd"}), wantErr: corefactory.ErrUnsupported},
		{name: "memory without section", cfg: storageCfg(&idempotencyconfig.StorageConfig{Type: idempotencyconfig.StorageTypeMemory}), wantErr: corefactory.ErrConfigRequired},
		{name: "redis not registered", cfg: storageCfg(&idempotencyconfig.StorageConfig{
			Type: idempotencyconfig.StorageTypeRedis, Redis: &storageconfig.RedisConfig{},
		}), wantErr: corefactory.ErrNotRegistered, wantMsg: "factory/reliability/idempotency/storages/redis"},
		{name: "nats not registered", cfg: storageCfg(&idempotencyconfig.StorageConfig{
			Type: idempotencyconfig.StorageTypeNATS, NATS: &storageconfig.NATSConfig{},
		}), wantErr: corefactory.ErrNotRegistered, wantMsg: "factory/reliability/idempotency/storages/nats"},
		{name: "mongo not registered", cfg: storageCfg(&idempotencyconfig.StorageConfig{
			Type: idempotencyconfig.StorageTypeMongo, Mongo: &idempotencyconfig.MongoStorageConfig{},
		}), wantErr: corefactory.ErrNotRegistered, wantMsg: "factory/reliability/idempotency/storages/mongo"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := factory.New(tc.cfg).Build(t.Context())
			require.ErrorIs(t, err, tc.wantErr)
			require.ErrorContains(t, err, tc.wantMsg)
		})
	}
}

func TestUseStorage(t *testing.T) {
	t.Parallel()
	cfg := storageCfg(&idempotencyconfig.StorageConfig{Type: idempotencyconfig.StorageTypeRedis})

	_, err := factory.New(cfg).UseStorage(idempotencyconfig.StorageTypeRedis, nil).Build(t.Context())
	require.Error(t, err)

	failing := func(*idempotencyconfig.Config) (storages.Storage, error) { return nil, errBuild }
	_, err = factory.New(cfg).UseStorage(idempotencyconfig.StorageTypeRedis, failing).Build(t.Context())
	require.ErrorIs(t, err, errBuild)

	want, err := memorystorage.New()
	require.NoError(t, err)
	var called bool
	registered := func(got *idempotencyconfig.Config) (storages.Storage, error) {
		require.Same(t, cfg, got)
		called = true
		return want, nil
	}
	keeper, err := factory.New(cfg).
		UseStorage(idempotencyconfig.StorageTypeRedis, failing).
		UseStorage(idempotencyconfig.StorageTypeRedis, registered).
		Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, keeper)
	require.True(t, called, "the later registration replaces the earlier one")

	ok, _, err := keeper.AttemptLock(t.Context(), "k")
	require.NoError(t, err)
	require.True(t, ok)
	ok, _, err = keeper.AttemptLock(t.Context(), "k")
	require.NoError(t, err)
	require.False(t, ok, "the keeper runs on the registered storage")
}

func TestBuildCustomBackend(t *testing.T) {
	t.Parallel()
	cfg := storageCfg(&idempotencyconfig.StorageConfig{Type: "custom"})

	build := func(*idempotencyconfig.Config) (storages.Storage, error) { return memorystorage.New() }
	b := factory.New(cfg).UseStorage("custom", build)
	cfg.Storage.SetRegisteredTypes(b.RegisteredStorageTypes())
	require.NoError(t, cfg.Validate())
	got, err := b.Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, got)
	_, err = factory.New(cfg).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrUnsupported)
}

func TestBuild_MemoryCleanupTaskID(t *testing.T) {
	t.Parallel()
	decode := func(id string) *idempotencyconfig.Config {
		storage := &idempotencyconfig.StorageConfig{}
		dec := yaml.NewDecoder(strings.NewReader("type: memory\nmemory:\n  cleanupSchedule: '@every 1m'\n  cleanupTaskID: " + id))
		dec.KnownFields(true)
		require.NoError(t, dec.Decode(storage))
		return storageCfg(storage)
	}
	scheduler := &taskRecorder{}
	build := func(cfg *idempotencyconfig.Config) error {
		_, err := factory.New(cfg).UseScheduler(scheduler).Build(t.Context())
		return err
	}
	require.NoError(t, build(decode("first-cleanup")))
	require.ErrorIs(t, build(decode("first-cleanup")), memorystorage.ErrDuplicateCleanupTaskID)
	require.NoError(t, build(decode("second-cleanup")))
	require.Len(t, scheduler.tasks, 2)
	require.Equal(t, "first-cleanup", scheduler.tasks[0].ID)
	require.Equal(t, "second-cleanup", scheduler.tasks[1].ID)
}
