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

	"github.com/altessa-s/go-atlas/coord/limiters"
	"github.com/altessa-s/go-atlas/coord/limiters/storages"
	"github.com/altessa-s/go-atlas/coord/limiters/tokenbucket"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	memorystorage "github.com/altessa-s/go-atlas/coord/limiters/storages/memory"
	factory "github.com/altessa-s/go-atlas/factory/coord/limiters/tokenbucket"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
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

func (r *taskRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.tasks)
}

func ipContext(t *testing.T, ip string) context.Context {
	t.Helper()
	return limiters.ContextWithClientIP(t.Context(), ip)
}

func requireIPExhaustsAfter(t *testing.T, l *tokenbucket.RuleLimiter, ip string, n int64) {
	t.Helper()
	for i := range n {
		info, err := l.Limit(ipContext(t, ip))
		require.NoError(t, err, "request %d", i+1)
		require.Equal(t, n, info.Limit)
	}
	_, err := l.Limit(ipContext(t, ip))
	require.ErrorIs(t, err, limiters.ErrLimitExceeded)
}

func TestBuild_TargetRulesOverrideDefault(t *testing.T) {
	t.Parallel()
	sched := &taskRecorder{}
	cfg := validConfig()
	cfg.Storage.Memory.CleanupSchedule = "@every 1m"
	cfg.Rules.Default.Limit = 5
	cfg.Rules.Targets = []limiterconfig.TokenBucketTargetRule{{Target: "10.1.0.0/16", Limit: 2, Period: time.Hour}}

	l, err := factory.New(cfg).
		UseLogger(slog.New(slog.DiscardHandler)).
		UseDefaultLogger().
		UseScheduler(sched).
		UseMeter(metricstest.NewTestCollector()).
		Build(t.Context())
	require.NoError(t, err)

	requireIPExhaustsAfter(t, l, "10.1.2.3", 2)
	info, err := l.Limit(ipContext(t, "192.168.0.1"))
	require.NoError(t, err)
	require.EqualValues(t, 5, info.Limit, "addresses outside the target use the default rule")
	require.Equal(t, 1, sched.count(), "the memory cleanup task must be registered")
}

func TestBuild_NoRulesUsesFactoryDefaults(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Rules = nil
	l, err := factory.New(cfg).Build(t.Context())
	require.NoError(t, err)
	info, err := l.Limit(ipContext(t, "10.0.0.1"))
	require.NoError(t, err)
	require.EqualValues(t, factory.DefaultLimit, info.Limit)
}

func TestBuild_StorageConfigErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		storage *storageconfig.BackendConfig
		wantErr error
	}{
		{name: "nil storage", storage: nil, wantErr: corefactory.ErrConfigRequired},
		{name: "unsupported", storage: &storageconfig.BackendConfig{Type: "etcd"}, wantErr: corefactory.ErrUnsupported},
		{name: "memory without section", storage: &storageconfig.BackendConfig{Type: storageconfig.BackendTypeMemory}, wantErr: corefactory.ErrConfigRequired},
		{name: "redis not registered", storage: &storageconfig.BackendConfig{Type: storageconfig.BackendTypeRedis}, wantErr: corefactory.ErrNotRegistered},
		{name: "nats not registered", storage: &storageconfig.BackendConfig{Type: storageconfig.BackendTypeNATS}, wantErr: corefactory.ErrNotRegistered},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := validConfig()
			cfg.Storage = tc.storage
			_, err := factory.New(cfg).Build(t.Context())
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestUseStorage(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Rules.Default.Limit = 2
	cfg.Storage = &storageconfig.BackendConfig{Type: storageconfig.BackendTypeNATS}
	cfg.NATSContention = &limiterconfig.NATSContention{MaxCASAttempts: 3}

	_, err := factory.New(cfg).UseStorage(storageconfig.BackendTypeNATS, nil).Build(t.Context())
	require.Error(t, err)

	failing := func(context.Context, *storageconfig.BackendConfig, *limiterconfig.NATSContention) (storages.Storage, error) {
		return nil, errBuild
	}
	_, err = factory.New(cfg).UseStorage(storageconfig.BackendTypeNATS, failing).Build(t.Context())
	require.ErrorIs(t, err, errBuild)

	registered := func(ctx context.Context, got *storageconfig.BackendConfig, contention *limiterconfig.NATSContention) (storages.Storage, error) {
		require.NotNil(t, ctx)
		require.Same(t, cfg.Storage, got)
		require.Same(t, cfg.NATSContention, contention)
		return memorystorage.New(), nil
	}
	l, err := factory.New(cfg).
		UseStorage(storageconfig.BackendTypeNATS, failing).
		UseStorage(storageconfig.BackendTypeNATS, registered).
		Build(t.Context())
	require.NoError(t, err)
	requireIPExhaustsAfter(t, l, "10.0.0.7", 2)
}

func TestBuild_NilConfig(t *testing.T) {
	t.Parallel()
	_, err := factory.New(nil).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)
}
