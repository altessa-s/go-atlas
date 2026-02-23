// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"
)

func BenchmarkStorage_UpsertTask(b *testing.B) {
	s := memory.New(100)
	ctx := b.Context()
	state := &scheduler.TaskState{
		ID:       "bench",
		Status:   scheduler.TaskStatusActive,
		Schedule: "@every 1h",
	}

	b.ResetTimer()
	for b.Loop() {
		_ = s.UpsertTask(ctx, state)
	}
}

func BenchmarkStorage_GetTask(b *testing.B) {
	s := memory.New(100)
	ctx := b.Context()
	_ = s.UpsertTask(ctx, &scheduler.TaskState{
		ID:       "bench",
		Status:   scheduler.TaskStatusActive,
		Schedule: "@every 1h",
	})

	b.ResetTimer()
	for b.Loop() {
		_, _ = s.GetTask(ctx, "bench")
	}
}

func BenchmarkStorage_AddHistory(b *testing.B) {
	s := memory.New(10000)
	ctx := b.Context()

	b.ResetTimer()
	for b.Loop() {
		_ = s.AddHistory(ctx, &scheduler.TaskHistory{
			ID:        "h1",
			TaskID:    "task-1",
			StartedAt: time.Now().Unix(),
			Success:   true,
		})
	}
}

func BenchmarkStorage_Tasks(b *testing.B) {
	s := memory.New(100)
	ctx := b.Context()
	for i := range 50 {
		_ = s.UpsertTask(ctx, &scheduler.TaskState{
			ID:       string(rune('a'+i%26)) + string(rune('0'+i/26)),
			Status:   scheduler.TaskStatusActive,
			Schedule: "@every 1h",
		})
	}

	b.ResetTimer()
	for b.Loop() {
		for range s.Tasks(ctx) {
		}
	}
}

func BenchmarkStorage_History(b *testing.B) {
	s := memory.New(1000)
	ctx := b.Context()
	for i := range 100 {
		_ = s.AddHistory(ctx, &scheduler.TaskHistory{
			ID:        string(rune('a' + i%26)),
			TaskID:    "task-1",
			StartedAt: int64(i * 100),
		})
	}

	b.ResetTimer()
	for b.Loop() {
		for range s.History(ctx, "task-1") {
		}
	}
}

func BenchmarkStorage_CleanupHistory(b *testing.B) {
	s := memory.New(1000)
	ctx := b.Context()
	now := time.Now()
	for i := range 100 {
		_ = s.AddHistory(ctx, &scheduler.TaskHistory{
			ID:      string(rune('a' + i%26)),
			TaskID:  "task-1",
			EndedAt: now.Add(-time.Duration(i) * time.Hour).Unix(),
		})
	}

	b.ResetTimer()
	for b.Loop() {
		_ = s.CleanupHistory(ctx, 24*time.Hour)
	}
}
