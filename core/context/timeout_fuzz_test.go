// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package context_test

import (
	"testing"
	"time"

	corecontext "github.com/altessa-s/go-atlas/core/context"
)

func FuzzApplyTimeout(f *testing.F) {
	f.Add(int64(0))           // No timeout
	f.Add(int64(time.Minute)) // Standard
	f.Add(int64(-1))          // Negative (usually treated as 0 or immediate expire by stdlib)

	f.Fuzz(func(t *testing.T, d int64) {
		ctx := t.Context()
		timeout := time.Duration(d)

		gotCtx, cancel := corecontext.ApplyTimeout(ctx, timeout)
		defer cancel()

		if timeout == 0 {
			if gotCtx != ctx {
				t.Error("Expected original context for zero timeout")
			}
		} else if timeout < 0 {
			// Negative timeout -> immediate cancellation (standard valid behavior)
			// Should NOT be original context
			if gotCtx == ctx {
				t.Error("Expected new context for negative timeout")
			}
			select {
			case <-gotCtx.Done():
			default:
				t.Error("Expected immediate cancellation for negative timeout")
			}
		} else {
			if _, ok := gotCtx.Deadline(); !ok {
				t.Error("Expected deadline for positive timeout")
			}
		}
	})
}
