// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/scheduler"
)

func FuzzTaskPriority_String(f *testing.F) {
	f.Add(int32(0))
	f.Add(int32(1))
	f.Add(int32(4))
	f.Add(int32(-1))
	f.Add(int32(100))

	f.Fuzz(func(t *testing.T, val int32) {
		p := scheduler.TaskPriority(val)
		_ = p.String()
	})
}
