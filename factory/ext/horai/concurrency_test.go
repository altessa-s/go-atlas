// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/factory/ext/horai"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
)

func TestBuildLimitFunc(t *testing.T) {
	t.Parallel()

	t.Run("static", func(t *testing.T) {
		t.Parallel()
		fn, err := factory.BuildLimitFunc(schedulerconfig.Concurrency{Strategy: schedulerconfig.ConcurrencyStatic, MaxTasks: 7})
		require.NoError(t, err)
		require.Equal(t, 7, fn())
	})

	t.Run("static without limit uses default", func(t *testing.T) {
		t.Parallel()
		fn, err := factory.BuildLimitFunc(schedulerconfig.Concurrency{})
		require.NoError(t, err)
		require.Nil(t, fn)
	})

	t.Run("environment", func(t *testing.T) {
		t.Parallel()
		fn, err := factory.BuildLimitFunc(schedulerconfig.Concurrency{
			Strategy: schedulerconfig.ConcurrencyEnvironment, Environment: concurrency.EnvironmentRateLimited,
		})
		require.NoError(t, err)
		require.Equal(t, concurrency.ConcurrencyForEnvironment(concurrency.EnvironmentRateLimited), fn())
	})

	t.Run("memory-aware", func(t *testing.T) {
		t.Parallel()
		_, err := factory.BuildLimitFunc(schedulerconfig.Concurrency{Strategy: schedulerconfig.ConcurrencyMemoryAware})
		require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)

		fn, err := factory.BuildLimitFunc(schedulerconfig.Concurrency{
			Strategy:    schedulerconfig.ConcurrencyMemoryAware,
			MemoryAware: &schedulerconfig.MemoryAwareConcurrency{LowMemoryMB: 1, MediumMemoryMB: 2, HighMemoryMB: 3},
		})
		require.NoError(t, err)
		require.Positive(t, fn())
	})

	t.Run("adaptive", func(t *testing.T) {
		t.Parallel()
		_, err := factory.BuildLimitFunc(schedulerconfig.Concurrency{Strategy: schedulerconfig.ConcurrencyAdaptive})
		require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)

		fn, err := factory.BuildLimitFunc(schedulerconfig.Concurrency{
			Strategy: schedulerconfig.ConcurrencyAdaptive,
			Adaptive: &schedulerconfig.AdaptiveConcurrency{MemoryLowThresholdMB: 1, MemoryMediumThresholdMB: 2, HighLoadThreshold: 0.8},
		})
		require.NoError(t, err)
		require.Positive(t, fn())
	})

	t.Run("unknown", func(t *testing.T) {
		t.Parallel()
		_, err := factory.BuildLimitFunc(schedulerconfig.Concurrency{Strategy: "greedy"})
		require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	})
}
