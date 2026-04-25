// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package signals_test

import (
	"context"
	"os"
	"syscall"
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/signals"
)

func noopHandler(_ context.Context, _ os.Signal) error { return nil }

func BenchmarkNew(b *testing.B) {
	var sink *signals.Signal
	for b.Loop() {
		sink = signals.New()
	}
	_ = sink
}

func BenchmarkAddHandler(b *testing.B) {
	sig := signals.New()
	for b.Loop() {
		sig.AddHandler(noopHandler, syscall.SIGUSR1)
	}
}

func BenchmarkAddHandlerWithPriority(b *testing.B) {
	sig := signals.New()
	for b.Loop() {
		sig.AddHandlerWithPriority(noopHandler, signals.PriorityNormal, syscall.SIGUSR1)
	}
}

func BenchmarkAddBroadcastHandler(b *testing.B) {
	sig := signals.New()
	for b.Loop() {
		sig.AddBroadcastHandler(noopHandler)
	}
}
