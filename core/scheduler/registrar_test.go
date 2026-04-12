// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/scheduler"
)

func TestTaskPriority_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		priority scheduler.TaskPriority
		expected string
	}{
		{"unspecified", scheduler.TaskPriorityUnspecified, "unknown"},
		{"low", scheduler.TaskPriorityLow, "low"},
		{"normal", scheduler.TaskPriorityNormal, "normal"},
		{"high", scheduler.TaskPriorityHigh, "high"},
		{"critical", scheduler.TaskPriorityCritical, "critical"},
		{"negative", scheduler.TaskPriority(-1), "unknown"},
		{"out of range", scheduler.TaskPriority(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, tt.priority.String(), "TaskPriority(%d).String()", tt.priority)
		})
	}
}

func TestTaskPriority_Values(t *testing.T) {
	t.Parallel()
	require.Equal(t, scheduler.TaskPriority(0), scheduler.TaskPriorityUnspecified)
	require.Equal(t, scheduler.TaskPriority(1), scheduler.TaskPriorityLow)
	require.Equal(t, scheduler.TaskPriority(2), scheduler.TaskPriorityNormal)
	require.Equal(t, scheduler.TaskPriority(3), scheduler.TaskPriorityHigh)
	require.Equal(t, scheduler.TaskPriority(4), scheduler.TaskPriorityCritical)
}

func TestTaskConfig_ZeroValue(t *testing.T) {
	t.Parallel()
	var cfg scheduler.TaskConfig

	require.Equal(t, "", cfg.ID)
	require.Equal(t, scheduler.TaskPriorityUnspecified, cfg.Priority)
	require.Equal(t, 0, int(cfg.Timeout))
	require.False(t, cfg.RunOnStart)
	require.Nil(t, cfg.Func)
	require.Nil(t, cfg.Meta)
}
