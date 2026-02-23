// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/scheduler"
)

func BenchmarkTaskPriority_String(b *testing.B) {
	p := scheduler.TaskPriorityNormal
	for b.Loop() {
		_ = p.String()
	}
}
