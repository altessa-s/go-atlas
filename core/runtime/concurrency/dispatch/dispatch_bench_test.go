// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dispatch_test

import (
	"context"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency/dispatch"
)

// benchSink is a zero-overhead [dispatch.Sink] that discards all batches.
type benchSink struct{}

func (benchSink) StoreBatch(_ context.Context, _ []int) error { return nil }

func newBenchEngine(b *testing.B) *dispatch.Engine[int] {
	b.Helper()
	eng, err := dispatch.NewEngine[int](benchSink{},
		dispatch.WithBufferSize[int](1<<14),
		dispatch.WithBatchSize[int](256),
		dispatch.WithFlushInterval[int](50*time.Millisecond),
		dispatch.WithWorkers[int](2),
	)
	if err != nil {
		b.Fatalf("new engine: %v", err)
	}
	if err := eng.Start(); err != nil {
		b.Fatalf("start: %v", err)
	}
	b.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = eng.Shutdown(ctx)
	})
	return eng
}

func BenchmarkEngineSubmit(b *testing.B) {
	eng := newBenchEngine(b)
	b.ResetTimer()
	for b.Loop() {
		eng.Submit(1)
	}
}

func BenchmarkEngineSubmitParallel(b *testing.B) {
	eng := newBenchEngine(b)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			eng.Submit(1)
		}
	})
}
