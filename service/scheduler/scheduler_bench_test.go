// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/service/scheduler"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

func BenchmarkScheduler_New(b *testing.B) {
	storage := mustNewMemory(b, 100)
	b.ResetTimer()
	for b.Loop() {
		_ = scheduler.New(storage, scheduler.WithTickInterval(time.Second))
	}
}

func BenchmarkScheduler_Register(b *testing.B) {
	storage := mustNewMemory(b, 100)
	s := scheduler.New(storage, scheduler.WithTickInterval(time.Hour))
	ctx := b.Context()
	if err := s.Start(ctx); err != nil {
		b.Fatal(err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	b.ResetTimer()
	for b.Loop() {
		_ = s.Register(ctx, corescheduler.TaskConfig{
			ID:       "bench-task",
			Schedule: "@every 1h",
			Func:     func(_ context.Context) error { return nil },
		})
	}
}

func BenchmarkMemoryStorage_UpsertTask(b *testing.B) {
	storage := mustNewMemory(b, 100)
	ctx := b.Context()
	state := &scheduler.TaskState{
		TaskSummary: scheduler.TaskSummary{
			ID:       "bench",
			Status:   scheduler.TaskStatusActive,
			Schedule: "@every 1h",
		},
	}

	b.ResetTimer()
	for b.Loop() {
		_ = storage.UpsertTask(ctx, state)
	}
}

func BenchmarkMemoryStorage_GetTask(b *testing.B) {
	storage := mustNewMemory(b, 100)
	ctx := b.Context()
	_ = storage.UpsertTask(ctx, &scheduler.TaskState{
		TaskSummary: scheduler.TaskSummary{
			ID:       "bench",
			Status:   scheduler.TaskStatusActive,
			Schedule: "@every 1h",
		},
	})

	b.ResetTimer()
	for b.Loop() {
		_, _ = storage.GetTask(ctx, "bench")
	}
}

func BenchmarkMemoryStorage_AddHistory(b *testing.B) {
	storage := mustNewMemory(b, 1000)
	ctx := b.Context()

	b.ResetTimer()
	for b.Loop() {
		_ = storage.AddHistory(ctx, &scheduler.TaskHistory{
			ID:        "h1",
			TaskID:    "t1",
			RunID:     "r1",
			StartedAt: time.Now().Unix(),
			Success:   true,
		})
	}
}

func BenchmarkMemoryStorage_Tasks(b *testing.B) {
	storage := mustNewMemory(b, 100)
	ctx := b.Context()
	for i := range 100 {
		_ = storage.UpsertTask(ctx, &scheduler.TaskState{
			TaskSummary: scheduler.TaskSummary{
				ID:       string(rune('a'+i%26)) + string(rune('0'+i/26)),
				Status:   scheduler.TaskStatusActive,
				Schedule: "@every 1h",
			},
		})
	}

	b.ResetTimer()
	for b.Loop() {
		for range storage.Tasks(ctx) {
		}
	}
}

func BenchmarkTaskStatus_String(b *testing.B) {
	statuses := []scheduler.TaskStatus{
		scheduler.TaskStatusActive,
		scheduler.TaskStatusPaused,
		scheduler.TaskStatusDisabled,
		scheduler.TaskStatusRunning,
		scheduler.TaskStatusUnspecified,
	}
	b.ResetTimer()
	for b.Loop() {
		for _, s := range statuses {
			_ = s.String()
		}
	}
}
