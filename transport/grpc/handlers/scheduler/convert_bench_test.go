// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"testing"

	sched "github.com/altessa-s/go-atlas/service/scheduler"
)

func BenchmarkTaskStateToMap(b *testing.B) {
	s := &sched.TaskState{
		TaskSummary: sched.TaskSummary{
			ID:       "task1",
			Status:   1,
			Priority: 2,
		},
		Meta: map[string]string{"k": "v"},
	}
	for b.Loop() {
		taskStateToMap(s)
	}
}

func BenchmarkTaskSummaryToMap(b *testing.B) {
	s := &sched.TaskSummary{ID: "task1", Status: 1}
	for b.Loop() {
		taskSummaryToMap(s)
	}
}
