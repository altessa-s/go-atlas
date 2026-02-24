// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

func FuzzScheduler_Register(f *testing.F) {
	f.Add("task-1", "@every 1s", "description")
	f.Add("", "@every 5m", "")
	f.Add("my-task", "invalid", "test")
	f.Add("task", "0 */5 * * * *", "every 5 min")

	f.Fuzz(func(t *testing.T, id, schedule, description string) {
		storage := memory.New(10)
		s := scheduler.New(storage, scheduler.WithTickInterval(time.Hour))
		ctx := t.Context()
		if err := s.Start(ctx); err != nil {
			t.Fatal(err)
		}
		defer func() {
			stopCtx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			_ = s.Stop(stopCtx)
		}()

		// Should not panic
		_ = s.Register(ctx, corescheduler.TaskConfig{
			ID:          id,
			Schedule:    schedule,
			Description: description,
			Func:        func(_ context.Context) error { return nil },
		})
	})
}

func FuzzMemoryStorage_UpsertAndGet(f *testing.F) {
	f.Add("task-1", "@every 1h", int32(1))
	f.Add("", "@every 5m", int32(0))
	f.Add("task-xyz", "0 0 * * * *", int32(3))

	f.Fuzz(func(t *testing.T, id, schedule string, status int32) {
		storage := memory.New(10)
		ctx := t.Context()

		state := &scheduler.TaskState{
			TaskSummary: scheduler.TaskSummary{
				ID:       id,
				Status:   scheduler.TaskStatus(status),
				Schedule: schedule,
			},
		}

		// Should not panic
		_ = storage.UpsertTask(ctx, state)

		got, err := storage.GetTask(ctx, id)
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

func FuzzTaskStatus_String(f *testing.F) {
	f.Add(int32(0))
	f.Add(int32(1))
	f.Add(int32(2))
	f.Add(int32(3))
	f.Add(int32(4))
	f.Add(int32(99))

	f.Fuzz(func(t *testing.T, s int32) {
		status := scheduler.TaskStatus(s)
		// Should not panic
		result := status.String()
		if result == "" {
			t.Error("String() returned empty")
		}
	})
}
