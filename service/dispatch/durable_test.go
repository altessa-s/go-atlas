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
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/service/dispatch"
)

func TestDurableRetriesBeyondVolatileBudget(t *testing.T) {
	t.Parallel()
	sink := &fakeSink{failN: 5}
	e := newRunningEngine(t, sink, dispatch.WithWorkers[int](1), dispatch.WithRetryAttempts[int](1), dispatch.WithRetryBackoff[int](time.Millisecond), dispatch.WithRetryMaxBackoff[int](time.Millisecond), dispatch.WithWAL[int](t.TempDir(), intCodec{}))
	require.True(t, e.Submit(42))
	testhelpers.WaitFor(t, 2*time.Second, func() bool { return len(sink.snapshot()) == 1 }, "durable retry stalled")
	require.NoError(t, e.Shutdown(t.Context()))
	require.Zero(t, e.WAL().Stats().Pending)
}
func TestDurableAdmissionExceedsMemoryQueue(t *testing.T) {
	t.Parallel()
	sink := &slowSink{release: make(chan struct{})}
	e := newRunningEngine(t, sink, dispatch.WithWorkers[int](1), dispatch.WithBufferSize[int](1), dispatch.WithBatchSize[int](1), dispatch.WithWAL[int](t.TempDir(), intCodec{}))
	for i := range 50 {
		tier, err := e.Enqueue(i)
		require.NoError(t, err)
		require.Equal(t, dispatch.Logged, tier)
	}
	require.Zero(t, e.Dropped())
	close(sink.release)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	require.NoError(t, e.Shutdown(ctx))
	require.Len(t, sink.seen, 50)
}
func TestShutdownReportsAndRetainsBacklog(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	e := newRunningEngine(t, &fakeSink{failN: 1000}, dispatch.WithWorkers[int](1), dispatch.WithRetryAttempts[int](1), dispatch.WithWAL[int](dir, intCodec{}))
	require.True(t, e.Submit(42))
	require.ErrorIs(t, e.Shutdown(t.Context()), dispatch.ErrBacklog)
	w, records, err := wal.Open(dir)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.NoError(t, w.Close())
}
func TestReplayDecodeErrorNeverAcknowledges(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w, _, err := wal.Open(dir)
	require.NoError(t, err)
	_, err = w.Append([]byte("malformed"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	e, err := dispatch.NewEngine[int](&fakeSink{}, dispatch.WithWAL[int](dir, intCodec{}))
	require.NoError(t, err)
	require.ErrorIs(t, e.Start(), dispatch.ErrReplayDecode)
	w, records, err := wal.Open(dir)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, []byte("malformed"), records[0].Payload)
	require.NoError(t, w.Close())
}
