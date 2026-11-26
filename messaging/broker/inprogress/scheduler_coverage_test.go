// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package inprogress_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/runtime/cycle"
	"github.com/altessa-s/go-atlas/messaging/broker/inprogress"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	schedtask "github.com/altessa-s/go-horai/task"
)

type captureRegistrar struct {
	mu   sync.Mutex
	cfgs []schedtask.Config
}

func (r *captureRegistrar) Register(_ context.Context, cfg schedtask.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfgs = append(r.cfgs, cfg)
	return nil
}

func (r *captureRegistrar) registered() []schedtask.Config {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]schedtask.Config(nil), r.cfgs...)
}

type countingHeartbeater struct {
	calls atomic.Int64
	err   error
}

func (h *countingHeartbeater) InProgress() error {
	h.calls.Add(1)
	return h.err
}

func TestNew_RegistersTickTaskWithScheduler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		schedule inprogress.Option
		want     string
	}{
		{name: "string schedule", schedule: inprogress.WithTickSchedule(" @every 1s "), want: "@every 1s"},
		{name: "pointer schedule", schedule: inprogress.WithTickSchedule(new("@every 2s")), want: "@every 2s"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			reg := &captureRegistrar{}
			col := metricstest.NewTestCollector()
			mgr := mustNew(t, inprogress.WithScheduler(reg), tc.schedule, inprogress.WithMeter(col))

			cfgs := reg.registered()
			require.Len(t, cfgs, 1)
			cfg := cfgs[0]
			require.True(t, strings.HasPrefix(cfg.ID, "broker-inprogress-heartbeat-"), cfg.ID)
			require.Equal(t, tc.want, cfg.Schedule)
			require.True(t, cfg.DisableHistory)
			require.NotNil(t, cfg.Func)

			require.ErrorIs(t, mgr.RunTickCycle(t.Context()), cycle.ErrManaged,
				"manual ticks are refused once the scheduler owns the cycle")

			ok := &countingHeartbeater{}
			failing := &countingHeartbeater{err: errors.New("nak")}
			stopOK := mgr.Register(ok, 0)
			defer stopOK()
			stopFailing := mgr.Register(failing, 0)
			defer stopFailing()

			require.NoError(t, cfg.Func(t.Context()))
			require.Equal(t, int64(1), ok.calls.Load())
			require.Equal(t, int64(1), failing.calls.Load())
			require.Equal(t, float64(1), metricstest.GetCounterValue(t, col, "test_broker_inprogress_heartbeats_sent_total"))
			require.Equal(t, float64(1), metricstest.GetCounterValue(t, col, "test_broker_inprogress_heartbeat_errors_total"))

			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			require.ErrorIs(t, cfg.Func(ctx), context.Canceled)
		})
	}
}

func TestNew_NoRegistrationWithoutSchedule(t *testing.T) {
	t.Parallel()

	reg := &captureRegistrar{}
	mgr := mustNew(t,
		inprogress.WithScheduler(reg),
		inprogress.WithTickSchedule(""),
		inprogress.WithTickSchedule((*string)(nil)),
		inprogress.WithTickSchedule(new("  ")),
		inprogress.WithScheduler(nil),
		inprogress.WithMeter(nil),
	)
	require.Empty(t, reg.registered())

	h := &countingHeartbeater{}
	stop := mgr.Register(h, time.Hour)
	defer stop()
	require.NoError(t, mgr.RunTickCycle(t.Context()))
	require.Zero(t, h.calls.Load(), "interval not elapsed")
}

type failingRegistrar struct{}

var errRegisterFailed = errors.New("register failed")

func (failingRegistrar) Register(context.Context, schedtask.Config) error {
	return errRegisterFailed
}

func TestNew_FailedRegistrationIsAnError(t *testing.T) {
	t.Parallel()

	mgr, err := inprogress.New(
		inprogress.WithScheduler(failingRegistrar{}),
		inprogress.WithTickSchedule("@every 1s"),
	)
	require.ErrorIs(t, err, errRegisterFailed)
	require.Nil(t, mgr)
}

func TestNew_TickTaskID(t *testing.T) {
	t.Parallel()

	reg := &captureRegistrar{}
	mustNew(t, inprogress.WithScheduler(reg), inprogress.WithTickSchedule("@every 1s"))
	mustNew(t, inprogress.WithScheduler(reg), inprogress.WithTickSchedule("@every 1s"))
	mustNew(t, inprogress.WithScheduler(reg), inprogress.WithTickSchedule("@every 1s"), inprogress.WithTickTaskID("orders-heartbeat"))

	cfgs := reg.registered()
	require.Len(t, cfgs, 3)
	require.NotEqual(t, cfgs[0].ID, cfgs[1].ID, "managers with default IDs must not share a scheduler task")
	require.Equal(t, "orders-heartbeat", cfgs[2].ID)
}
