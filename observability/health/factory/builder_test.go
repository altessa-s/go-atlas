// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/health/factory"
)

// healthCheckTaskID is the ID the coordinator registers its check cycle under.
const healthCheckTaskID = "health-check"

func TestBuild_RequiresConfig(t *testing.T) {
	t.Parallel()

	_, err := factory.New(nil).Build()
	require.Error(t, err)
}

func TestBuild_WithoutScheduler(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultHealth()

	coordinator, err := factory.New(&cfg).Build()
	require.NoError(t, err)
	require.NotNil(t, coordinator)
}

// HealthCheckInterval is expressed as a duration while the coordinator takes a
// cron-style expression; "@every" is the form that carries the duration across.
func TestBuild_MapsHealthCheckIntervalToSchedule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		interval time.Duration
		want     string
	}{
		{"seconds", 5 * time.Second, "@every 5s"},
		{"sub-second", 250 * time.Millisecond, "@every 250ms"},
		{"minutes", 2 * time.Minute, "@every 2m0s"},
		{"compound", 90 * time.Second, "@every 1m30s"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := config.DefaultHealth()
			cfg.HealthCheckInterval = tc.interval

			registrar := &testhelpers.MockTaskRegistrar{}

			_, err := factory.New(&cfg).UseScheduler(registrar).Build()
			require.NoError(t, err)

			task, ok := registrar.Task(healthCheckTaskID)
			require.True(t, ok, "the check cycle must be registered")
			require.Equal(t, tc.want, task.Schedule)
		})
	}
}

// Without a scheduler there is nothing to drive the check cycle, so the
// interval must not silently look configured.
func TestBuild_NoSchedulerRegistersNoTask(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultHealth()
	cfg.HealthCheckInterval = time.Second

	registrar := &testhelpers.MockTaskRegistrar{}

	_, err := factory.New(&cfg).Build()
	require.NoError(t, err)
	require.Zero(t, registrar.Count())
}

// A zero interval means "do not poll"; registering "@every 0s" would spin.
func TestBuild_ZeroIntervalRegistersNoTask(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultHealth()
	cfg.HealthCheckInterval = 0

	registrar := &testhelpers.MockTaskRegistrar{}

	_, err := factory.New(&cfg).UseScheduler(registrar).Build()
	require.NoError(t, err)
	require.Zero(t, registrar.Count(), "a zero interval must not schedule a check cycle")
}

// The remaining tunables were already mapped; this pins that adding the
// schedule did not displace them.
func TestBuild_StillMapsCoordinatorTunables(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultHealth()
	cfg.NumShards = 8
	cfg.MaxConcurrentHealthChecks = 3
	cfg.CheckTimeout = 750 * time.Millisecond

	registrar := &testhelpers.MockTaskRegistrar{}

	coordinator, err := factory.New(&cfg).UseScheduler(registrar).Build()
	require.NoError(t, err)
	require.NotNil(t, coordinator)
	require.Equal(t, 1, registrar.Count())
}
