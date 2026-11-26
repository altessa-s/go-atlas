// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package inprogress_test

import (
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/messaging/broker/inprogress"
)

type mockHeartbeater struct {
	count atomic.Int32
}

// mustNew creates a Manager and fails the test when New returns an error.
func mustNew(tb testing.TB, opts ...inprogress.Option) *inprogress.Manager {
	tb.Helper()
	mgr, err := inprogress.New(opts...)
	require.NoError(tb, err)
	return mgr
}

func (m *mockHeartbeater) InProgress() error {
	m.count.Add(1)
	return nil
}

func TestNew_Default(t *testing.T) {
	mgr := mustNew(t)
	require.NotNil(t, mgr, "New() returned nil")
}

func TestNew_WithLogger(t *testing.T) {
	mgr := mustNew(t, inprogress.WithLogger(nil))
	require.NotNil(t, mgr, "New(WithLogger) returned nil")
}

func TestManager_RegisterAndStop(t *testing.T) {
	mgr := mustNew(t)
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
	mgr := mustNew(t)

	fn := mgr.RegisterTickSchedulerFunc()
	require.NotNil(t, fn, "RegisterTickSchedulerFunc() returned nil")

	// After registering scheduler func, direct RunTickCycle should return error
	err := mgr.RunTickCycle(t.Context())
	require.NotNil(t, err, "RunTickCycle() should return error when scheduler-managed")
}

func TestManager_MultipleHeartbeaters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mgr := mustNew(t)
		h1 := &mockHeartbeater{}
		h2 := &mockHeartbeater{}

		stop1 := mgr.Register(h1, 10*time.Millisecond)
		stop2 := mgr.Register(h2, 10*time.Millisecond)

		time.Sleep(15 * time.Millisecond)
		_ = mgr.RunTickCycle(t.Context())

		require.Positive(t, h1.count.Load())
		require.Positive(t, h2.count.Load())

		stop1()
		stop2()
	})
}
