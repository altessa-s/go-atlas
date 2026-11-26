// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/encoding/serializer"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
	"github.com/altessa-s/go-atlas/reliability/saga"

	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
	sagafactory "github.com/altessa-s/go-atlas/factory/reliability/saga"
)

type staticLeader bool

func (l staticLeader) IsLeader() bool { return bool(l) }

var errDeclined = errors.New("card declined")

// Injected dependencies reach the orchestrator: the retry predicate stops a
// failing pivot step after one attempt, the dead-letter callback fires, and
// the recovery cycle is registered with the scheduler.
func TestBuild_InjectedDependenciesAreWired(t *testing.T) {
	t.Parallel()
	var chargeCalls atomic.Int32
	def := saga.NewDefinition[order]("pivot").
		Step("reserve", func(context.Context, *order) error { return nil }).
		Compensate(func(context.Context, *order) error { return nil }).
		Step("charge", func(context.Context, *order) error {
			chargeCalls.Add(1)
			return errDeclined
		}).Pivot().
		MustBuild()

	cfg := sagaconfig.Default()
	cfg.MaxStepAttempts = 5
	cfg.StepRetryBaseDelay = time.Millisecond
	cfg.StepRetryMaxDelay = time.Millisecond

	var dead atomic.Pointer[saga.Instance]
	reg := &testhelpers.MockTaskRegistrar{}
	orch, err := sagafactory.New(&cfg, def).
		UseLogger(slog.New(slog.DiscardHandler)).
		UseDefaultLogger().
		UseMeter(metricstest.NewTestCollector()).
		UseScheduler(reg).
		UseLeaderElector(staticLeader(true)).
		UseSerializer(&serializer.JSON{}).
		UseOnDeadLetter(func(inst *saga.Instance) { dead.Store(inst) }).
		UseShouldRetry(func(err error) bool { return !errors.Is(err, errDeclined) }).
		Build(t.Context())
	require.NoError(t, err)

	inst, err := orch.Start(t.Context(), "o1", order{})
	require.ErrorIs(t, err, errDeclined)
	require.Equal(t, saga.StatusFailed, inst.Status)
	require.EqualValues(t, 1, chargeCalls.Load(), "a non-retryable error must not be retried")
	require.NotNil(t, dead.Load(), "the dead-letter callback must fire")

	tasks := reg.Tasks()
	require.Len(t, tasks, 2, "retention is on by default")
	require.Equal(t, saga.RecoveryTaskIDPrefix+"pivot", tasks[0].ID)
	require.Equal(t, cfg.RecoverySchedule, tasks[0].Schedule)
}

// Retention registers its own per-definition task only when it is enabled.
func TestBuild_RetentionRegistration(t *testing.T) {
	t.Parallel()
	def := saga.NewDefinition[order]("pivot").
		Step("a", func(context.Context, *order) error { return nil }).ReadOnly().
		MustBuild()

	for _, tc := range []struct {
		name      string
		retention time.Duration
		tasks     int
	}{
		{"off", 0, 1},
		{"default", sagaconfig.Default().Retention, 2},
		{"on", time.Hour, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := sagaconfig.Default()
			cfg.Retention = tc.retention
			reg := &testhelpers.MockTaskRegistrar{}
			_, err := sagafactory.New(&cfg, def).UseScheduler(reg).Build(t.Context())
			require.NoError(t, err)

			tasks := reg.Tasks()
			require.Len(t, tasks, tc.tasks)
			if tc.retention > 0 {
				_, ok := reg.Task(saga.RetentionTaskIDPrefix + "pivot")
				require.True(t, ok)
			}
		})
	}
}

func TestBuild_RecoveryRegistrationFailure(t *testing.T) {
	t.Parallel()
	boom := errors.New("registrar down")
	cfg := sagaconfig.Default()
	_, err := sagafactory.New(&cfg, orderDef()).UseScheduler(&testhelpers.MockTaskRegistrar{Err: boom}).Build(t.Context())
	require.ErrorIs(t, err, boom)
}
