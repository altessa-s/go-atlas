// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memcleanup_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/runtime/cycle"
	"github.com/altessa-s/go-atlas/internal/memcleanup"

	schedtask "github.com/altessa-s/go-horai/task"
)

type recordingRegistrar struct {
	cfgs []schedtask.Config
}

func (r *recordingRegistrar) Register(_ context.Context, cfg schedtask.Config) error {
	r.cfgs = append(r.cfgs, cfg)
	return nil
}

func TestRegister(t *testing.T) {
	t.Parallel()

	var (
		calls int
		reg   recordingRegistrar
	)
	task := cycle.New(func(context.Context) error { calls++; return nil })

	err := memcleanup.Register(t.Context(), &reg, task, schedtask.Config{
		ID:       "cleanup",
		Schedule: "@every 1m",
	})
	require.NoError(t, err)
	require.Len(t, reg.cfgs, 1)
	require.Equal(t, "cleanup", reg.cfgs[0].ID)
	require.Equal(t, "@every 1m", reg.cfgs[0].Schedule)
	require.True(t, task.IsManaged())

	require.NoError(t, reg.cfgs[0].Func(t.Context()))
	require.Equal(t, 1, calls, "registered func must run the cycle")
	require.ErrorIs(t, task.Run(t.Context()), cycle.ErrManaged)
	require.Equal(t, 1, calls, "manual run must be refused once scheduler-managed")
}

func TestRegister_Rejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		registrar schedtask.Registrar
		schedule  string
		want      error
	}{
		{name: "nil registrar", registrar: nil, schedule: "@every 1m", want: memcleanup.ErrNoScheduler},
		{name: "typed nil registrar", registrar: (*recordingRegistrar)(nil), schedule: "@every 1m", want: memcleanup.ErrNoScheduler},
		{name: "empty schedule", registrar: &recordingRegistrar{}, schedule: "", want: memcleanup.ErrNoSchedule},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			task := cycle.New(func(context.Context) error { return nil })
			err := memcleanup.Register(t.Context(), tc.registrar, task, schedtask.Config{
				ID: "cleanup", Schedule: tc.schedule,
			})
			require.ErrorIs(t, err, tc.want)
			require.False(t, task.IsManaged(), "a rejected registration must leave manual runs allowed")
		})
	}
}
