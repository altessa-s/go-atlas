// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters/memory"
)

type bufferedCounterAdapter struct {
	*memory.Adapter
	recording, releaseRecord chan struct{}
	flushing, releaseFlush   chan struct{}
	pending, flushed         atomic.Int32
}

func (a *bufferedCounterAdapter) RecordCounter(name string, labels map[string]string, delta float64) {
	if delta == 1 {
		close(a.recording)
		<-a.releaseRecord
	}
	a.pending.Add(int32(delta))
}

func (a *bufferedCounterAdapter) Flush() error {
	a.flushed.Add(a.pending.Swap(0))
	close(a.flushing)
	<-a.releaseFlush
	return nil
}

func TestCollector_ShutdownDrainsAcceptedObservations(t *testing.T) {
	t.Parallel()

	a := &bufferedCounterAdapter{
		Adapter: memory.New(), recording: make(chan struct{}), releaseRecord: make(chan struct{}),
		flushing: make(chan struct{}), releaseFlush: make(chan struct{}),
	}
	c := metrics.New(metrics.WithAdapter(a))
	ctr := metrics.MustCounter(c, metrics.MetricOpts{Name: "events"})
	recorded := make(chan struct{})
	go func() {
		defer close(recorded)
		ctr.Inc()
	}()
	<-a.recording

	shutdown := make(chan error, 1)
	go func() { shutdown <- c.Shutdown(t.Context()) }()
	// If Shutdown flushes early, it has already missed the parked observation.
	select {
	case <-a.flushing:
	case <-time.After(50 * time.Millisecond):
	}
	close(a.releaseRecord)
	<-recorded
	<-a.flushing
	ctr.Add(2) // Admission must already be closed while the final flush runs.
	close(a.releaseFlush)
	require.NoError(t, <-shutdown)
	require.Equal(t, int32(1), a.flushed.Load())
	require.Zero(t, a.pending.Load())
}
