// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/limiters/budget"
	"github.com/altessa-s/go-atlas/factory/coord/limiters/budget"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	schedtask "github.com/altessa-s/go-horai/task"
)

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

func budgetCfg(limit int64, backend *limiterconfig.StorageConfig) *limiterconfig.Budget {
	return &limiterconfig.Budget{Limit: limit, Period: time.Hour, Storage: backend}
}

// requireExhaustsAfter asserts l allows exactly n calls for key.
func requireExhaustsAfter(t *testing.T, l *budget.Limiter, key string, n int) {
	t.Helper()
	for i := range n {
		require.NoError(t, l.Allow(t.Context(), key), "call %d is within budget", i+1)
	}
	require.ErrorIs(t, l.Allow(t.Context(), key), budget.ErrBudgetExhausted)
}

func TestBuild_MemoryEnforcesBudgetAndRegistersCleanup(t *testing.T) {
	t.Parallel()
	sched := &taskRecorder{}
	tc := metricstest.NewTestCollector()
	l, err := factory.New(budgetCfg(2, &limiterconfig.StorageConfig{
		Type:   limiterconfig.StorageTypeMemory,
		Memory: &storageconfig.MemoryConfig{CleanupSchedule: "@every 1m"},
	})).UseScheduler(sched).UseMeter(tc).Build(t.Context())
	require.NoError(t, err)

	requireExhaustsAfter(t, l, "tenant-a", 2)
	require.NoError(t, l.Allow(t.Context(), "tenant-b"), "budgets are per key")
	require.Equal(t, 1, sched.count(), "the memory cleanup task must be registered")
}

func TestBuild_MissingMemorySection(t *testing.T) {
	t.Parallel()
	_, err := factory.New(budgetCfg(1, &limiterconfig.StorageConfig{Type: limiterconfig.StorageTypeMemory})).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)
}

func TestBuild_InvalidLimitIsRejected(t *testing.T) {
	t.Parallel()
	_, err := factory.New(budgetCfg(0, &limiterconfig.StorageConfig{
		Type: limiterconfig.StorageTypeMemory, Memory: &storageconfig.MemoryConfig{},
	})).Build(t.Context())
	require.ErrorIs(t, err, budget.ErrInvalidLimit)
}
