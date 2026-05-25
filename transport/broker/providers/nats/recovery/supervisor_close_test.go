// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// TestSupervisor_Close_WaitsForInFlightRecoveries is the regression
// guard for the WaitGroup wiring. Before the fix Close() only flipped
// the cancel signal — callers had no way to know whether the recovery
// goroutines had actually stopped touching state. The new contract is
// "Close returns ⇒ no recovery goroutine is still running"; this test
// proves it by spawning a recovery whose handler blocks on a channel,
// asking Close to return, then asserting the handler observed the
// cancel and exited cleanly before Close returned.
func TestSupervisor_Close_WaitsForInFlightRecoveries(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()

	// Register a stream with a resubscribe handler that blocks until
	// the test releases it OR the supervisor's context cancels. This
	// keeps the recovery goroutine alive long enough for the test to
	// reason about Close's behavior.
	const stream = "stream-1"
	const consumer = "consumer-1"

	gate := make(chan struct{})
	var handlerExited atomic.Bool
	registry.RegisterStream(stream, jetstream.StreamConfig{Name: stream}, RecoveryStrategyAuto)
	registry.RegisterResubscribeHandler(stream, consumer, func() error {
		// Wait for either the test gate or the supervisor ctx to fire.
		// The recovery loop wraps this in coreretry.Do, so ctx
		// cancellation propagates here as the abort signal.
		<-gate
		handlerExited.Store(true)
		return nil
	})

	sup := NewSupervisor(SupervisorConfig{
		Registry:    registry,
		Logger:      slog.New(slog.DiscardHandler),
		MaxAttempts: 1,
		Backoff:     time.Millisecond,
	})

	sup.RecoverConsumer(stream, consumer, nil)

	// Give the goroutine a moment to enter the handler before we close
	// — without this, Close races the goroutine startup and the test
	// proves nothing about the WaitGroup wiring.
	time.Sleep(20 * time.Millisecond)

	// Release the handler concurrently with Close so the goroutine
	// completes naturally. Close MUST not return until that completion
	// is observed.
	done := make(chan struct{})
	go func() {
		close(gate)
		close(done)
	}()

	closeReturned := make(chan struct{})
	go func() {
		sup.Close()
		close(closeReturned)
	}()

	select {
	case <-closeReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return within 2s — WaitGroup is not tracking the recovery goroutine")
	}
	<-done

	require.True(t, handlerExited.Load(),
		"handler must have observed completion before Close returned — Close must wait for in-flight recoveries")
}
