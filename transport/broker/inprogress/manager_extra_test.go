// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package inprogress_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
	require.NotNil(t, mgr, "New() returned nil")
}

func TestNew_WithLogger(t *testing.T) {
	mgr := inprogress.New(inprogress.WithLogger(nil))
	require.NotNil(t, mgr, "New(WithLogger) returned nil")
}

func TestManager_RegisterAndStop(t *testing.T) {
	mgr := inprogress.New()
	h := &mockHeartbeater{}

	stop := mgr.Register(h, 50*time.Millisecond)
	require.NotNil(t, stop, "Register() returned nil stop func")

	// Run a tick to trigger heartbeat
	_ = mgr.RunTickCycle(t.Context())
	_ = mgr.RunTickCycle(t.Context())

	stop()
	// After stop, tick should not call heartbeater
	countBefore := h.count.Load()
	_ = mgr.RunTickCycle(t.Context())
	countAfter := h.count.Load()

	require.Equal(t, countBefore, countAfter)
}

func TestManager_RegisterTickSchedulerFunc(t *testing.T) {
	mgr := inprogress.New()

	fn := mgr.RegisterTickSchedulerFunc()
	require.NotNil(t, fn, "RegisterTickSchedulerFunc() returned nil")

	// After registering scheduler func, direct RunTickCycle should return error
	err := mgr.RunTickCycle(t.Context())
	require.NotNil(t, err, "RunTickCycle() should return error when scheduler-managed")
}

func TestManager_MultipleHeartbeaters(t *testing.T) {
	mgr := inprogress.New()
	h1 := &mockHeartbeater{}
	h2 := &mockHeartbeater{}

	stop1 := mgr.Register(h1, 10*time.Millisecond)
	stop2 := mgr.Register(h2, 10*time.Millisecond)

	time.Sleep(15 * time.Millisecond)
	_ = mgr.RunTickCycle(t.Context())

	require.NotEqual(t, 0, h1.count.Load())
	require.NotEqual(t, 0, h2.count.Load())

	stop1()
	stop2()
}
