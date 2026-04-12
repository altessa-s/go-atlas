// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package signals_test

import (
	"context"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/runtime/signals"
)

func TestSignal_AddHandler_Start_Stop(t *testing.T) {
	var called atomic.Bool

	s := signals.New(
		signals.WithSignals(syscall.SIGUSR1),
		signals.WithHandlerTimeout(2*time.Second),
		signals.WithShutdownTimeout(2*time.Second),
	)

	s.AddHandler(func(ctx context.Context, sig os.Signal) error {
		called.Store(true)
		return nil
	}, syscall.SIGUSR1)

	s.Start()

	// Send signal to self
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGUSR1)

	// Give handler time to run
	time.Sleep(200 * time.Millisecond)

	require.NoError(t, s.Stop())
	require.True(t, called.Load(), "handler was not called")
}

func TestSignal_AddHandlerWithPriority(t *testing.T) {
	var order []int
	s := signals.New(
		signals.WithSignals(syscall.SIGUSR2),
		signals.WithExecutionMode(signals.SequentialMode),
		signals.WithHandlerTimeout(2*time.Second),
	)

	s.AddHandlerWithPriority(func(_ context.Context, _ os.Signal) error {
		order = append(order, 2)
		return nil
	}, signals.PriorityLow, syscall.SIGUSR2)

	s.AddHandlerWithPriority(func(_ context.Context, _ os.Signal) error {
		order = append(order, 1)
		return nil
	}, signals.PriorityHigh, syscall.SIGUSR2)

	s.Start()

	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGUSR2)

	time.Sleep(200 * time.Millisecond)
	_ = s.Stop()

	require.Len(t, order, 2)
	// Higher priority should run first
	require.Equal(t, 1, order[0], "expected high priority first, got order %v", order)
}

func TestSignal_BroadcastHandler(t *testing.T) {
	var called atomic.Bool
	s := signals.New(
		signals.WithSignals(syscall.SIGUSR1),
		signals.WithHandlerTimeout(2*time.Second),
	)

	s.AddBroadcastHandler(func(_ context.Context, _ os.Signal) error {
		called.Store(true)
		return nil
	})

	s.Start()

	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGUSR1)

	time.Sleep(200 * time.Millisecond)
	_ = s.Stop()

	require.True(t, called.Load(), "broadcast handler was not called")
}

func TestSignal_Shutdown(t *testing.T) {
	s := signals.New(
		signals.WithSignals(syscall.SIGUSR1),
		signals.WithShutdownTimeout(1*time.Second),
	)
	s.Start()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	require.NoError(t, s.Shutdown(ctx))
}

func TestSignal_ParallelMode(t *testing.T) {
	s := signals.New(
		signals.WithSignals(syscall.SIGUSR1),
		signals.WithExecutionMode(signals.ParallelMode),
		signals.WithWorkerPoolSize(5),
		signals.WithHandlerTimeout(2*time.Second),
	)

	var count atomic.Int32
	for range 3 {
		s.AddHandler(func(_ context.Context, _ os.Signal) error {
			count.Add(1)
			return nil
		}, syscall.SIGUSR1)
	}

	s.Start()

	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGUSR1)

	time.Sleep(300 * time.Millisecond)
	_ = s.Stop()

	require.Equal(t, int32(3), count.Load(), "expected 3 handlers called in parallel")
}

func TestSignal_ErrorHandler(t *testing.T) {
	var errCalled atomic.Bool
	s := signals.New(
		signals.WithSignals(syscall.SIGUSR1),
		signals.WithHandlerTimeout(50*time.Millisecond),
		signals.WithErrorHandler(func(sig os.Signal, err error) {
			errCalled.Store(true)
		}),
	)

	s.AddHandler(func(_ context.Context, _ os.Signal) error {
		time.Sleep(500 * time.Millisecond) // will timeout
		return nil
	}, syscall.SIGUSR1)

	s.Start()

	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGUSR1)

	time.Sleep(300 * time.Millisecond)
	_ = s.Stop()
}
