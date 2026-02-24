// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"
)

func FuzzStorage_UpsertAndGet(f *testing.F) {
	f.Add("task-1", int32(1), "@every 1h")
	f.Add("", int32(0), "")
	f.Add("task-abc", int32(4), "0 0 * * * *")

	f.Fuzz(func(t *testing.T, id string, status int32, schedule string) {
		s := memory.New(10)
		ctx := t.Context()

		state := &scheduler.TaskState{
			TaskSummary: scheduler.TaskSummary{
				ID:       id,
				Status:   scheduler.TaskStatus(status),
				Schedule: schedule,
			},
		}

		// Should not panic
		_ = s.UpsertTask(ctx, state)

		got, err := s.GetTask(ctx, id)
		if err != nil {
			t.Fatalf("GetTask error: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil state")
		}
		if got.ID != id {
			t.Errorf("ID=%q, want %q", got.ID, id)
		}
	})
}

func FuzzStorage_AddHistory(f *testing.F) {
	f.Add("h1", "task-1", "r1", int64(100), true, "")
	f.Add("", "", "", int64(0), false, "some error")

	f.Fuzz(func(t *testing.T, id, taskID, runID string, startedAt int64, success bool, errMsg string) {
		s := memory.New(5)
		ctx := t.Context()

		h := &scheduler.TaskHistory{
			ID:        id,
			TaskID:    taskID,
			RunID:     runID,
			StartedAt: startedAt,
			Success:   success,
			Error:     errMsg,
		}

		// Should not panic
		_ = s.AddHistory(ctx, h)

		for range s.History(ctx, taskID) {
		}
	})
}
