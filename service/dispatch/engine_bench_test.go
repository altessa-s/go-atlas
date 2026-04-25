// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dispatch_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/io/wal"
	"github.com/altessa-s/go-atlas/service/dispatch"
)

// nopSink is a zero-allocation sink used by the submit-path benchmarks.
// It discards the batch so the measurement isolates the engine's hot
// path (channel send, batching, worker wake-up) from any downstream
// I/O cost.
type nopSink struct{}

func (nopSink) StoreBatch(_ context.Context, _ []int) error { return nil }

// newBenchEngine constructs a running Engine with the given options and
// wires graceful Shutdown into b.Cleanup. The flush interval is long
// enough that batching is triggered by batch size rather than the
// timer, which keeps benchmarks deterministic.
func newBenchEngine(b *testing.B, opts ...dispatch.Option[int]) *dispatch.Engine[int] {
	b.Helper()
	base := []dispatch.Option[int]{
		dispatch.WithBatchSize[int](256),
		dispatch.WithFlushInterval[int](time.Hour),
		dispatch.WithWorkers[int](2),
		dispatch.WithBufferSize[int](1 << 16),
	}
	eng, err := dispatch.NewEngine[int](nopSink{}, append(base, opts...)...)
	require.NoError(b, err)
	require.NoError(b, eng.Start())
	b.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = eng.Shutdown(ctx)
	})
	return eng
}

// BenchmarkEngine_Submit_NoWAL measures the pure in-memory submit hot
// path: no encode, no WAL append, just the channel send + atomic
// accounting. This is the number 99% of callers care about.
func BenchmarkEngine_Submit_NoWAL(b *testing.B) {
	eng := newBenchEngine(b)

	b.ReportAllocs()
	for b.Loop() {
		eng.Submit(1)
	}
}

// BenchmarkEngine_Submit_NoWAL_Parallel measures the contended case —
// many producers pushing into the same engine. This stresses the
// buffered-channel send under concurrency and surfaces any scaling
// ceilings in the dispatch hot path.
func BenchmarkEngine_Submit_NoWAL_Parallel(b *testing.B) {
	eng := newBenchEngine(b, dispatch.WithWorkers[int](4))

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			eng.Submit(1)
		}
	})
}

// BenchmarkEngine_Submit_WithWAL includes the codec encode + WAL
// Append on the hot path. The difference between this and
// BenchmarkEngine_Submit_NoWAL is the cost of durability.
func BenchmarkEngine_Submit_WithWAL(b *testing.B) {
	dir := b.TempDir()
	eng := newBenchEngine(b,
		dispatch.WithWAL[int](dir, intCodec{},
			wal.WithMaxSegmentBytes(64<<20),
			// Long interval so the fsync goroutine doesn't dominate.
			wal.WithFsyncInterval(time.Hour),
		),
	)

	b.ReportAllocs()
	for b.Loop() {
		eng.Submit(1)
	}
}

// BenchmarkEngine_Submit_Drop measures the drop path: the buffer is
// sized to 1, one record fills it, and every subsequent Submit takes
// the default-branch (dropItem + OnDrop). Use this to understand the
// worst-case cost of back-pressure-less submission under a saturated
// downstream.
func BenchmarkEngine_Submit_Drop(b *testing.B) {
	// Block the sink so the buffer fills immediately and stays full.
	blocker := make(chan struct{})
	defer close(blocker)

	sink := benchBlockingSink{ch: blocker}
	eng, err := dispatch.NewEngine[int](sink,
		dispatch.WithBufferSize[int](1),
		dispatch.WithBatchSize[int](1),
		dispatch.WithFlushInterval[int](time.Hour),
		dispatch.WithWorkers[int](1),
	)
	require.NoError(b, err)
	require.NoError(b, eng.Start())
	b.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = eng.Shutdown(ctx)
	})

	// Prime the buffer so the first Submit lands and the second starts
	// dropping.
	eng.Submit(0)
	eng.Submit(0)

	b.ReportAllocs()
	for b.Loop() {
		eng.Submit(1)
	}
}

// benchBlockingSink stalls StoreBatch on ch until the benchmark ends,
// guaranteeing the dispatcher's internal buffer stays full.
type benchBlockingSink struct{ ch chan struct{} }

func (b benchBlockingSink) StoreBatch(ctx context.Context, _ []int) error {
	select {
	case <-b.ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
