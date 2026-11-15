// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package schedulerconfig_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/runtime/concurrency"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func concurrencyFieldErrors(t *testing.T, err error) validation.Errors {
	t.Helper()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	return verrs
}

func TestConcurrency_Validate(t *testing.T) {
	t.Parallel()

	def := schedulerconfig.DefaultConcurrency()
	require.Equal(t, schedulerconfig.ConcurrencyStatic, def.Strategy)
	require.Equal(t, 5, def.MaxTasks)
	require.NoError(t, def.Validate())

	tests := []struct {
		name  string
		cfg   schedulerconfig.Concurrency
		field string
	}{
		{"missing strategy", schedulerconfig.Concurrency{}, "Strategy"},
		{"unknown strategy", schedulerconfig.Concurrency{Strategy: "greedy"}, "Strategy"},
		{"negative max tasks", schedulerconfig.Concurrency{Strategy: schedulerconfig.ConcurrencyStatic, MaxTasks: -1}, "MaxTasks"},
		{"unknown environment", schedulerconfig.Concurrency{
			Strategy: schedulerconfig.ConcurrencyEnvironment, Environment: "quantum",
		}, "Environment"},
		{"memory-aware without section", schedulerconfig.Concurrency{Strategy: schedulerconfig.ConcurrencyMemoryAware}, "MemoryAware"},
		{"memory-aware with zero thresholds", schedulerconfig.Concurrency{
			Strategy: schedulerconfig.ConcurrencyMemoryAware, MemoryAware: &schedulerconfig.MemoryAwareConcurrency{LowMemoryMB: 1},
		}, "MemoryAware"},
		{"adaptive without section", schedulerconfig.Concurrency{Strategy: schedulerconfig.ConcurrencyAdaptive}, "Adaptive"},
		{"adaptive with zero thresholds", schedulerconfig.Concurrency{
			Strategy: schedulerconfig.ConcurrencyAdaptive, Adaptive: &schedulerconfig.AdaptiveConcurrency{},
		}, "Adaptive"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Contains(t, concurrencyFieldErrors(t, tc.cfg.Validate()), tc.field)
		})
	}

	require.NoError(t, (&schedulerconfig.Concurrency{
		Strategy: schedulerconfig.ConcurrencyAdaptive,
		Adaptive: &schedulerconfig.AdaptiveConcurrency{MemoryLowThresholdMB: 100, MemoryMediumThresholdMB: 500},
	}).Validate())
}

func TestConcurrency_BuildLimitFunc(t *testing.T) {
	t.Parallel()

	t.Run("static", func(t *testing.T) {
		t.Parallel()
		fn, err := (&schedulerconfig.Concurrency{Strategy: schedulerconfig.ConcurrencyStatic, MaxTasks: 7}).BuildLimitFunc()
		require.NoError(t, err)
		require.Equal(t, 7, fn())
	})

	t.Run("static without limit uses default", func(t *testing.T) {
		t.Parallel()
		fn, err := (&schedulerconfig.Concurrency{}).BuildLimitFunc()
		require.NoError(t, err)
		require.Nil(t, fn)
	})

	t.Run("environment", func(t *testing.T) {
		t.Parallel()
		fn, err := (&schedulerconfig.Concurrency{
			Strategy: schedulerconfig.ConcurrencyEnvironment, Environment: concurrency.EnvironmentRateLimited,
		}).BuildLimitFunc()
		require.NoError(t, err)
		require.Equal(t, concurrency.ConcurrencyForEnvironment(concurrency.EnvironmentRateLimited), fn())
	})

	t.Run("memory-aware", func(t *testing.T) {
		t.Parallel()
		_, err := (&schedulerconfig.Concurrency{Strategy: schedulerconfig.ConcurrencyMemoryAware}).BuildLimitFunc()
		require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)

		fn, err := (&schedulerconfig.Concurrency{
			Strategy:    schedulerconfig.ConcurrencyMemoryAware,
			MemoryAware: &schedulerconfig.MemoryAwareConcurrency{LowMemoryMB: 1, MediumMemoryMB: 2, HighMemoryMB: 3},
		}).BuildLimitFunc()
		require.NoError(t, err)
		require.Positive(t, fn())
	})

	t.Run("adaptive", func(t *testing.T) {
		t.Parallel()
		_, err := (&schedulerconfig.Concurrency{Strategy: schedulerconfig.ConcurrencyAdaptive}).BuildLimitFunc()
		require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)

		fn, err := (&schedulerconfig.Concurrency{
			Strategy: schedulerconfig.ConcurrencyAdaptive,
			Adaptive: &schedulerconfig.AdaptiveConcurrency{MemoryLowThresholdMB: 1, MemoryMediumThresholdMB: 2, HighLoadThreshold: 0.8},
		}).BuildLimitFunc()
		require.NoError(t, err)
		require.Positive(t, fn())
	})

	t.Run("unknown", func(t *testing.T) {
		t.Parallel()
		_, err := (&schedulerconfig.Concurrency{Strategy: "greedy"}).BuildLimitFunc()
		require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	})
}

func TestStorageDefaults(t *testing.T) {
	t.Parallel()

	mongo := schedulerconfig.DefaultStorageMongoConfig()
	require.NotEmpty(t, mongo.TasksCollection)
	require.NotEmpty(t, mongo.HistoryCollection)

	redis := schedulerconfig.DefaultStorageRedisConfig()
	require.NotEmpty(t, redis.KeyPrefix)
	require.Positive(t, redis.MaxHistoryPerTask)

	require.Positive(t, schedulerconfig.DefaultStorageMemoryConfig().MaxHistoryPerTask)

	storage := schedulerconfig.DefaultStorageConfig()
	require.Equal(t, schedulerconfig.StorageTypeMemory, storage.Type)
	require.NoError(t, storage.Validate())

	storage.Type = "etcd"
	require.Contains(t, concurrencyFieldErrors(t, storage.Validate()), "Type")

	storage = schedulerconfig.StorageConfig{Type: schedulerconfig.StorageTypeMongo, Mongo: &mongo}
	require.NoError(t, storage.Validate())
}
