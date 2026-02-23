// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package inprogress

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

type mockHeartbeater struct {
	calls atomic.Int64
	err   error
}

func (m *mockHeartbeater) InProgress() error {
	m.calls.Add(1)
	return m.err
}

func TestManager_RunTickCycle_SendsInProgress_Repeated(t *testing.T) {
	mgr := New()

	h := &mockHeartbeater{}
	stop := mgr.Register(h, 20*time.Millisecond)
	defer stop()

	ctx := t.Context()

	// Run tick cycles in a loop to simulate scheduler behavior
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				close(done)
				return
			case <-ticker.C:
				_ = mgr.RunTickCycle(ctx)
			}
		}
	}()

	testhelpers.WaitFor(t, 2*time.Second, func() bool {
		return h.calls.Load() >= 1
	}, "timed out waiting for InProgress call")
}

func TestManager_RunTickCycle_RespectsInterval_Repeated(t *testing.T) {
	mgr := New()

	fast := &mockHeartbeater{}
	slow := &mockHeartbeater{}
	mgr.Register(fast, 15*time.Millisecond)
	mgr.Register(slow, 5*time.Second)

	ctx := t.Context()

	// Run tick cycles in a loop to simulate scheduler behavior
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				close(done)
				return
			case <-ticker.C:
				_ = mgr.RunTickCycle(ctx)
			}
		}
	}()

	testhelpers.WaitFor(t, 2*time.Second, func() bool {
		return fast.calls.Load() >= 3
	}, "timed out waiting for fast heartbeater calls")

	if slow.calls.Load() > 0 {
		t.Fatalf("expected slow heartbeater not to be called, got %d", slow.calls.Load())
	}
}

func TestManager_StopFunc(t *testing.T) {
	mgr := New()

	h := &mockHeartbeater{}
	stop := mgr.Register(h, 15*time.Millisecond)

	ctx := t.Context()

	// Run tick cycles in a loop to simulate scheduler behavior
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				close(done)
				return
			case <-ticker.C:
				_ = mgr.RunTickCycle(ctx)
			}
		}
	}()

	// Wait until at least one call is made.
	testhelpers.WaitFor(t, 2*time.Second, func() bool {
		return h.calls.Load() >= 1
	}, "timed out waiting for initial InProgress call")

	stop()
	callsAfterStop := h.calls.Load()

	// Give enough time for several would-be ticks.
	time.Sleep(100 * time.Millisecond)

	if h.calls.Load() > callsAfterStop {
		t.Fatalf("expected no calls after stop, got %d additional", h.calls.Load()-callsAfterStop)
	}
}

func TestManager_StopIsIdempotent(t *testing.T) {
	mgr := New()

	h := &mockHeartbeater{}
	stop := mgr.Register(h, time.Second)

	stop()
	stop() // must not panic
	stop()
}

func TestManager_ErrorRespectsInterval(t *testing.T) {
	mgr := New()

	h := &mockHeartbeater{err: errors.New("connection lost")}
	mgr.Register(h, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())

	// Run tick cycles in a loop to simulate scheduler behavior
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				close(done)
				return
			case <-ticker.C:
				_ = mgr.RunTickCycle(ctx)
			}
		}
	}()

	// Wait long enough for multiple intervals.
	time.Sleep(350 * time.Millisecond)
	cancel()
	<-done

	// With 100ms interval over 350ms, expect ~3 calls max.
	// Without the fix, tickInterval=10ms would cause ~35 calls.
	calls := h.calls.Load()
	if calls > 5 {
		t.Fatalf("expected calls to respect interval even on error, got %d (retry storm)", calls)
	}
}

func TestManager_ConcurrentRegisterStop(t *testing.T) {
	mgr := New()

	ctx, cancel := context.WithCancel(t.Context())

	// Run tick cycles in a loop to simulate scheduler behavior
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				close(done)
				return
			case <-ticker.C:
				_ = mgr.RunTickCycle(ctx)
			}
		}
	}()

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			h := &mockHeartbeater{}
			stop := mgr.Register(h, 10*time.Millisecond)
			time.Sleep(time.Duration(10+h.calls.Load()) * time.Millisecond)
			stop()
		})
	}

	wg.Wait()
	cancel()
	<-done
}

func TestManager_RunTickCycle_SendsInProgress(t *testing.T) {
	mgr := New()

	h := &mockHeartbeater{}
	stop := mgr.Register(h, 10*time.Millisecond)
	defer stop()

	// Wait for the interval to pass so the heartbeater becomes due
	time.Sleep(15 * time.Millisecond)

	// Run a single tick cycle
	err := mgr.RunTickCycle(t.Context())
	if err != nil {
		t.Fatalf("RunTickCycle returned error: %v", err)
	}

	// Wait a bit for the heartbeat to be sent
	time.Sleep(5 * time.Millisecond)

	if h.calls.Load() == 0 {
		t.Fatal("expected InProgress to be called, but it wasn't")
	}
}

func TestManager_RunTickCycle_RespectsContextCancel(t *testing.T) {
	mgr := New()

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Cancel immediately

	err := mgr.RunTickCycle(ctx)
	if err == nil {
		t.Fatal("expected RunTickCycle to return error when context is canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled error, got: %v", err)
	}
}

func TestManager_RunTickCycle_RespectsInterval(t *testing.T) {
	mgr := New()

	fast := &mockHeartbeater{}
	slow := &mockHeartbeater{}
	mgr.Register(fast, 10*time.Millisecond)
	mgr.Register(slow, 5*time.Second)

	// Wait a bit to ensure entries are registered
	time.Sleep(5 * time.Millisecond)

	// Run multiple tick cycles
	for range 3 {
		err := mgr.RunTickCycle(t.Context())
		if err != nil {
			t.Fatalf("RunTickCycle returned error: %v", err)
		}
		time.Sleep(15 * time.Millisecond)
	}

	if fast.calls.Load() == 0 {
		t.Fatal("expected fast heartbeater to be called")
	}

	if slow.calls.Load() > 0 {
		t.Fatalf("expected slow heartbeater not to be called, got %d calls", slow.calls.Load())
	}
}
