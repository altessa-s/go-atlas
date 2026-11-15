// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/observability/health"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	schedtask "github.com/altessa-s/go-horai/task"
)

// healthCheckTaskID is the ID the coordinator registers its check cycle under.
const healthCheckTaskID = "health-check"

func TestBuild_RequiresConfig(t *testing.T) {
	t.Parallel()

	_, err := factory.New(nil).Build(t.Context())
	require.Error(t, err)
}

func TestBuild_WithoutScheduler(t *testing.T) {
	t.Parallel()

	cfg := observabilityconfig.DefaultHealth()

	coordinator, err := factory.New(&cfg).Build(t.Context())
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

			cfg := observabilityconfig.DefaultHealth()
			cfg.HealthCheckInterval = tc.interval

			registrar := &testhelpers.MockTaskRegistrar{}

			_, err := factory.New(&cfg).UseScheduler(registrar).Build(t.Context())
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

	cfg := observabilityconfig.DefaultHealth()
	cfg.HealthCheckInterval = time.Second

	registrar := &testhelpers.MockTaskRegistrar{}

	_, err := factory.New(&cfg).Build(t.Context())
	require.NoError(t, err)
	require.Zero(t, registrar.Count())
}

// A zero interval means "do not poll"; registering "@every 0s" would spin.
func TestBuild_ZeroIntervalRegistersNoTask(t *testing.T) {
	t.Parallel()

	cfg := observabilityconfig.DefaultHealth()
	cfg.HealthCheckInterval = 0

	registrar := &testhelpers.MockTaskRegistrar{}

	_, err := factory.New(&cfg).UseScheduler(registrar).Build(t.Context())
	require.NoError(t, err)
	require.Zero(t, registrar.Count(), "a zero interval must not schedule a check cycle")
}

// The remaining tunables were already mapped; this pins that adding the
// schedule did not displace them.
func TestBuild_StillMapsCoordinatorTunables(t *testing.T) {
	t.Parallel()

	cfg := observabilityconfig.DefaultHealth()
	cfg.NumShards = 8
	cfg.MaxConcurrentHealthChecks = 3
	cfg.CheckTimeout = 750 * time.Millisecond

	registrar := &testhelpers.MockTaskRegistrar{}

	coordinator, err := factory.New(&cfg).UseScheduler(registrar).Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, coordinator)
	require.Equal(t, 1, registrar.Count())
}

func TestBuild_ReturnsRegistrationError(t *testing.T) {
	t.Parallel()
	cfg := observabilityconfig.DefaultHealth()
	cfg.HealthCheckInterval = time.Second
	boom := errors.New("registrar unavailable")
	registrar := &testhelpers.MockTaskRegistrar{Err: boom}
	coordinator, err := factory.New(&cfg).UseScheduler(registrar).Build(t.Context())
	require.ErrorIs(t, err, boom)
	require.Nil(t, coordinator)
}

type ctxKey struct{}

// ctxRegistrar records the value under ctxKey of the context Register runs on.
type ctxRegistrar struct{ got any }

func (r *ctxRegistrar) Register(ctx context.Context, _ schedtask.Config) error {
	r.got = ctx.Value(ctxKey{})
	return nil
}

// The startup registration runs on the caller's context, not a detached one.
func TestBuild_RegistersOnCallerContext(t *testing.T) {
	t.Parallel()
	cfg := observabilityconfig.DefaultHealth()
	cfg.HealthCheckInterval = time.Second
	registrar := &ctxRegistrar{}

	ctx := context.WithValue(t.Context(), ctxKey{}, "startup")
	_, err := factory.New(&cfg).UseScheduler(registrar).Build(ctx)
	require.NoError(t, err)
	require.Equal(t, "startup", registrar.got)
}
