// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"testing"

	sched "github.com/altessa-s/go-atlas/service/scheduler"
)

func TestTaskStateToMap(t *testing.T) {
	s := &sched.TaskState{
		ID:          "task1",
		Description: "desc",
		Status:      1,
		Priority:    2,
		Schedule:    "*/5 * * * *",
		LastRunAt:   1000,
		NextRunAt:   2000,
		LastRunID:   "run1",
		Failures:    3,
		SkipNextRun: true,
		Meta:        map[string]string{"key": "val"},
		CreatedAt:   100,
		UpdatedAt:   200,
	}

	m := taskStateToMap(s)
	if m["id"] != "task1" {
		t.Fatalf("id = %v", m["id"])
	}
	if m["status"] != int64(1) {
		t.Fatalf("status = %v", m["status"])
	}
	if m["failures"] != int64(3) {
		t.Fatalf("failures = %v", m["failures"])
	}
	if m["skipNextRun"] != true {
		t.Fatalf("skipNextRun = %v", m["skipNextRun"])
	}
	meta, ok := m["meta"].(map[string]any)
	if !ok || meta["key"] != "val" {
		t.Fatalf("meta = %v", m["meta"])
	}
}

func TestTaskSummaryToMap(t *testing.T) {
	s := &sched.TaskSummary{
		ID:       "task2",
		Status:   2,
		Priority: 1,
		Failures: 5,
	}

	m := taskSummaryToMap(s)
	if m["id"] != "task2" {
		t.Fatalf("id = %v", m["id"])
	}
	if m["failures"] != int64(5) {
		t.Fatalf("failures = %v", m["failures"])
	}
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
	if m["id"] != "hist1" {
		t.Fatalf("id = %v", m["id"])
	}
	if m["taskId"] != "task1" {
		t.Fatalf("taskId = %v", m["taskId"])
	}
	if m["success"] != true {
		t.Fatalf("success = %v", m["success"])
	}
	if m["durationMs"] != int64(1000) {
		t.Fatalf("durationMs = %v", m["durationMs"])
	}
}
