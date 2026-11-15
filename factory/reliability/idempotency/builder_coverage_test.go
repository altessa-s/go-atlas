// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/reliability/idempotency"
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
		Memory: &storageconfig.MemoryConfig{CleanupSchedule: "@every 1m"},
	})

	keeper, err := factory.New(cfg).
		UseLogger(slog.New(slog.DiscardHandler)).
		UseScheduler(sched).
		UseMeter(tc).
		Build()
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
			_, err := factory.New(tc.cfg).Build()
			require.ErrorIs(t, err, tc.wantErr)
			require.ErrorContains(t, err, tc.wantMsg)
		})
	}
}

func TestUseStorage(t *testing.T) {
	t.Parallel()
	cfg := storageCfg(&idempotencyconfig.StorageConfig{Type: idempotencyconfig.StorageTypeRedis})

	_, err := factory.New(cfg).UseStorage(idempotencyconfig.StorageTypeRedis, nil).Build()
	require.Error(t, err)

	failing := func(*idempotencyconfig.Config) (storages.Storage, error) { return nil, errBuild }
	_, err = factory.New(cfg).UseStorage(idempotencyconfig.StorageTypeRedis, failing).Build()
	require.ErrorIs(t, err, errBuild)

	want := memorystorage.New()
	var called bool
	registered := func(got *idempotencyconfig.Config) (storages.Storage, error) {
		require.Same(t, cfg, got)
		called = true
		return want, nil
	}
	keeper, err := factory.New(cfg).
		UseStorage(idempotencyconfig.StorageTypeRedis, failing).
		UseStorage(idempotencyconfig.StorageTypeRedis, registered).
		Build()
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
