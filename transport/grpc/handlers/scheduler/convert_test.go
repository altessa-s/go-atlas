// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"testing"

	"github.com/stretchr/testify/require"

	sched "github.com/altessa-s/go-atlas/service/scheduler"
)

func TestTaskStateToMap(t *testing.T) {
	s := &sched.TaskState{
		TaskSummary: sched.TaskSummary{
			ID:          "task1",
			Description: "desc",
			Status:      1,
			Priority:    2,
			Schedule:    "*/5 * * * *",
			LastRunAt:   1000,
			NextRunAt:   2000,
			Failures:    3,
			SkipNextRun: true,
		},
		LastRunID: "run1",
		Meta:      map[string]string{"key": "val"},
		CreatedAt: 100,
		UpdatedAt: 200,
	}

	m := taskStateToMap(s)
	require.Equal(t, "task1", m["id"])
	require.Equal(t, int64(1), m["status"])
	require.Equal(t, int64(3), m["failures"])
	require.Equal(t, true, m["skipNextRun"])
	meta, ok := m["meta"].(map[string]any)
	require.True(t, ok, "meta = %v", m["meta"])
	require.Equal(t, "val", meta["key"])
}

func TestTaskSummaryToMap(t *testing.T) {
	s := &sched.TaskSummary{
		ID:       "task2",
		Status:   2,
		Priority: 1,
		Failures: 5,
	}

	m := taskSummaryToMap(s)
	require.Equal(t, "task2", m["id"])
	require.Equal(t, int64(5), m["failures"])
}

func TestTaskHistoryToMap(t *testing.T) {
	h := &sched.TaskHistory{
		ID:         "hist1",
		TaskID:     "task1",
		RunID:      "run1",
		StartedAt:  1000,
		EndedAt:    2000,
		DurationMs: 1000,
		Success:    true,
		Error:      "",
	}

	m := taskHistoryToMap(h)
	require.Equal(t, "hist1", m["id"])
	require.Equal(t, "task1", m["taskId"])
	require.Equal(t, true, m["success"])
	require.Equal(t, int64(1000), m["durationMs"])
}
