// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package inprogress

import (
	"testing"
	"time"
)

func FuzzManager_Register(f *testing.F) {
	f.Add(int64(10))
	f.Add(int64(100))
	f.Add(int64(1))

	f.Fuzz(func(t *testing.T, intervalMs int64) {
		if intervalMs <= 0 {
			return
		}
		mgr := New()
		h := &fuzzHeartbeater{}
		stop := mgr.Register(h, time.Duration(intervalMs)*time.Millisecond)
		stop()
		stop() // idempotent
	})
}

type fuzzHeartbeater struct{}

func (f *fuzzHeartbeater) InProgress() error { return nil }
