// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/health"
)

func TestRegistrationFailureCanBeRetried(t *testing.T) {
	t.Parallel()
	boom := errors.New("registration failed")
	registrar := &testhelpers.MockTaskRegistrar{Err: boom}
	coordinator := health.New(health.WithScheduler(registrar), health.WithCheckSchedule("@every 1m"))
	t.Cleanup(coordinator.Close)
	require.Zero(t, registrar.Count(), "construction must have no scheduling side effects")
	require.ErrorIs(t, coordinator.RegisterHealthChecks(t.Context()), boom)
	require.NoError(t, coordinator.RunHealthCheckCycle(t.Context()))
	registrar.Err = nil
	require.NoError(t, coordinator.RegisterHealthChecks(t.Context()))
	require.ErrorIs(t, coordinator.RunHealthCheckCycle(t.Context()), health.ErrSchedulerManaged)
	require.NoError(t, coordinator.RegisterHealthChecks(t.Context()))
	require.Equal(t, 2, registrar.Count(), "successful registration is idempotent")
	task, ok := registrar.Task("health-check")
	require.True(t, ok)
	require.NoError(t, task.Func(t.Context()))
}
