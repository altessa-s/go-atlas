// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"testing"

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
			if got := tt.priority.String(); got != tt.expected {
				t.Errorf("TaskPriority(%d).String() = %q, want %q", tt.priority, got, tt.expected)
			}
		})
	}
}

func TestTaskPriority_Values(t *testing.T) {
	t.Parallel()
	if scheduler.TaskPriorityUnspecified != 0 {
		t.Errorf("TaskPriorityUnspecified = %d, want 0", scheduler.TaskPriorityUnspecified)
	}
	if scheduler.TaskPriorityLow != 1 {
		t.Errorf("TaskPriorityLow = %d, want 1", scheduler.TaskPriorityLow)
	}
	if scheduler.TaskPriorityNormal != 2 {
		t.Errorf("TaskPriorityNormal = %d, want 2", scheduler.TaskPriorityNormal)
	}
	if scheduler.TaskPriorityHigh != 3 {
		t.Errorf("TaskPriorityHigh = %d, want 3", scheduler.TaskPriorityHigh)
	}
	if scheduler.TaskPriorityCritical != 4 {
		t.Errorf("TaskPriorityCritical = %d, want 4", scheduler.TaskPriorityCritical)
	}
}

func TestTaskConfig_ZeroValue(t *testing.T) {
	t.Parallel()
	var cfg scheduler.TaskConfig

	if cfg.ID != "" {
		t.Errorf("zero TaskConfig.ID = %q, want empty", cfg.ID)
	}
	if cfg.Priority != scheduler.TaskPriorityUnspecified {
		t.Errorf("zero TaskConfig.Priority = %d, want Unspecified", cfg.Priority)
	}
	if cfg.Timeout != 0 {
		t.Errorf("zero TaskConfig.Timeout = %v, want 0", cfg.Timeout)
	}
	if cfg.RunOnStart {
		t.Error("zero TaskConfig.RunOnStart = true, want false")
	}
	if cfg.Func != nil {
		t.Error("zero TaskConfig.Func should be nil")
	}
	if cfg.Meta != nil {
		t.Error("zero TaskConfig.Meta should be nil")
	}
}
