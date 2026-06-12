// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/core/scheduler"
)

func BenchmarkTaskPriority_String(b *testing.B) {
	p := scheduler.TaskPriorityNormal
	for b.Loop() {
		_ = p.String()
	}
}

func BenchmarkTaskConfig_Construction(b *testing.B) {
	for b.Loop() {
		cfg := scheduler.TaskConfig{
			ID:       "bench-task",
			Schedule: "@every 5m",
			Func:     func(ctx context.Context) error { return nil },
			Priority: scheduler.TaskPriorityNormal,
			Meta: map[string]string{
				"team":  "platform",
				"owner": "bench",
			},
		}
		_ = cfg
	}
}
