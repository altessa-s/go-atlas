// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package inprogress

import (
	"testing"
	"time"
)

func BenchmarkManager_Register(b *testing.B) {
	mgr := New()
	h := &benchHeartbeater{}
	for b.Loop() {
		stop := mgr.Register(h, time.Second)
		stop()
	}
}

func BenchmarkManager_RunTickCycle(b *testing.B) {
	mgr := New()
	h := &benchHeartbeater{}
	mgr.Register(h, time.Millisecond)
	ctx := b.Context()

	for b.Loop() {
		mgr.RunTickCycle(ctx)
	}
}

type benchHeartbeater struct{}

func (b *benchHeartbeater) InProgress() error { return nil }
