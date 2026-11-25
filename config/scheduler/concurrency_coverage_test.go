// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package schedulerconfig_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

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
