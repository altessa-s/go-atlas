// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package signals_test

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/runtime/signals"
)

func TestSignal_AddBroadcastHandlerWithPriority(t *testing.T) {
	called := make(chan struct{}, 1)

	sig := signals.New(
		signals.WithSignals(syscall.SIGUSR1),
		signals.WithSignalChannelBuffer(1),
		signals.WithHandlerTimeout(2*time.Second),
	)

	sig.AddBroadcastHandlerWithPriority(func(ctx context.Context, s os.Signal) error {
		called <- struct{}{}
		return nil
	}, signals.PriorityHighest)

	sig.Start()
	defer sig.Stop()

	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGUSR1)

	select {
	case <-called:
	case <-time.After(3 * time.Second):
		require.Fail(t, "handler not called within timeout")
	}
}

func TestSignal_AddBroadcastHandlerWithPriority_NilHandler(t *testing.T) {
	sig := signals.New(signals.WithSignals(syscall.SIGUSR2))
	sig.AddBroadcastHandlerWithPriority(nil, signals.PriorityNormal)
}

func TestWithSignalChannelBuffer(t *testing.T) {
	sig := signals.New(
		signals.WithSignals(syscall.SIGUSR2),
		signals.WithSignalChannelBuffer(5),
	)
	sig.Start()
	sig.Stop()
}
