// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package inprogress_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/transport/broker/inprogress"
)

type mockHeartbeater struct {
	count atomic.Int32
}

func (m *mockHeartbeater) InProgress() error {
	m.count.Add(1)
	return nil
}

func TestNew_Default(t *testing.T) {
	mgr := inprogress.New()
	if mgr == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_WithLogger(t *testing.T) {
	mgr := inprogress.New(inprogress.WithLogger(nil))
	if mgr == nil {
		t.Fatal("New(WithLogger) returned nil")
	}
}

func TestManager_RegisterAndStop(t *testing.T) {
	mgr := inprogress.New()
	h := &mockHeartbeater{}

	stop := mgr.Register(h, 50*time.Millisecond)
	if stop == nil {
		t.Fatal("Register() returned nil stop func")
	}

	// Run a tick to trigger heartbeat
	_ = mgr.RunTickCycle(t.Context())
	_ = mgr.RunTickCycle(t.Context())

	stop()
	// After stop, tick should not call heartbeater
	countBefore := h.count.Load()
	_ = mgr.RunTickCycle(t.Context())
	countAfter := h.count.Load()

	if countAfter != countBefore {
		t.Error("heartbeater called after stop")
	}
}

func TestManager_RegisterTickSchedulerFunc(t *testing.T) {
	mgr := inprogress.New()

	fn := mgr.RegisterTickSchedulerFunc()
	if fn == nil {
		t.Fatal("RegisterTickSchedulerFunc() returned nil")
	}

	// After registering scheduler func, direct RunTickCycle should return error
	err := mgr.RunTickCycle(t.Context())
	if err == nil {
		t.Error("RunTickCycle() should return error when scheduler-managed")
	}
}

func TestManager_MultipleHeartbeaters(t *testing.T) {
	mgr := inprogress.New()
	h1 := &mockHeartbeater{}
	h2 := &mockHeartbeater{}

	stop1 := mgr.Register(h1, 10*time.Millisecond)
	stop2 := mgr.Register(h2, 10*time.Millisecond)

	time.Sleep(15 * time.Millisecond)
	_ = mgr.RunTickCycle(t.Context())

	if h1.count.Load() == 0 {
		t.Error("h1 not called")
	}
	if h2.count.Load() == 0 {
		t.Error("h2 not called")
	}

	stop1()
	stop2()
}
