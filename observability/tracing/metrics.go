// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
)

const exporterLabel = "exporter"

type exporterCounters struct {
	rejected metrics.Counter
	failures metrics.Counter
}

type exporterOutcome struct {
	rejected int
	failed   bool
}

type exportMetrics struct {
	exported     metrics.Counter
	dropped      metrics.Counter
	perExporter  []exporterCounters
	length       metrics.Gauge
	queueLength  func() int
	pendingDrops atomic.Uint64
	mu           sync.Mutex
	disabled     bool // protected by mu
	lastLength   int
	done         chan struct{}
}

func newExportMetrics(ctx context.Context, meter metrics.Meter, exporters []adapters.Adapter, queueLength func() int) *exportMetrics {
	if meter == nil || metrics.IsNoop(meter) {
		return nil
	}
	m := metrics.MustMeter{Meter: meter.WithSubsystem("tracing")}
	rejected := m.MustCounter(metrics.MetricOpts{
		Name:       "spans_rejected_total",
		Help:       "Spans rejected by the collector in partial-success responses.",
		LabelNames: []string{exporterLabel},
	})
	failures := m.MustCounter(metrics.MetricOpts{
		Name:       "export_failures_total",
		Help:       "Failed batch export calls.",
		LabelNames: []string{exporterLabel},
	})

	perExporter := make([]exporterCounters, len(exporters))
	for i, exp := range exporters {
		lbl := metrics.Labels{exporterLabel: exp.Name()}
		perExporter[i] = exporterCounters{
			rejected: rejected.WithLabels(lbl),
			failures: failures.WithLabels(lbl),
		}
	}

	stats := &exportMetrics{
		exported: m.MustCounter(metrics.MetricOpts{
			Name: "spans_exported_total",
			Help: "Spans accepted by every exporter.",
		}),
		dropped: m.MustCounter(metrics.MetricOpts{
			Name: "spans_dropped_total",
			Help: "Sampled spans dropped because the export queue was full or the shutdown deadline expired.",
		}),
		perExporter: perExporter,
		length:      m.MustGauge(metrics.MetricOpts{Name: "queue_length", Help: "Spans waiting in the export queue."}),
		queueLength: queueLength,
		done:        make(chan struct{}),
	}
	// Keep queue pressure observable even during a stalled export, without
	// calling user-supplied metric instruments from Span.End.
	go func() {
		defer close(stats.done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				stats.sample()
			case <-ctx.Done():
				return
			}
		}
	}()
	return stats
}

// disable stops metric writers without draining batches (used when a secondary
// concurrent Shutdown caller cancels while the primary shutdown is still live).
func (m *exportMetrics) disable() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.disabled {
		return
	}
	m.disabled = true
	if m.lastLength != 0 {
		m.length.Add(-float64(m.lastLength))
		m.lastLength = 0
	}
}

// abandonAndDisable atomically claims any remaining waiting/inflight batches via
// drain, records all pending and abandoned drops, zeroes queue_length, and
// fences further metric writes. Lock order: m.mu -> p.mu (same as sample).
func (m *exportMetrics) abandonAndDisable(drain func() uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	extraDrops := drain()
	if m.disabled {
		return
	}
	if drops := m.pendingDrops.Swap(0) + extraDrops; drops > 0 {
		m.dropped.Add(float64(drops))
	}
	if m.lastLength != 0 {
		m.length.Add(-float64(m.lastLength))
		m.lastLength = 0
	}
	m.disabled = true
}

// completeBatch transitions state from batchInflight to batchDone and records
// per-exporter outcomes while holding m.mu, preventing double counting against
// concurrent shutdown deadline abandonment.
func (m *exportMetrics) completeBatch(state *atomic.Int32, acceptedByAll int, outcomes []exporterOutcome) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.disabled {
		return
	}
	if !state.CompareAndSwap(batchInflight, batchDone) {
		return
	}
	if acceptedByAll > 0 {
		m.exported.Add(float64(acceptedByAll))
	}
	for i, o := range outcomes {
		if o.failed {
			m.perExporter[i].failures.Inc()
		} else if o.rejected > 0 {
			m.perExporter[i].rejected.Add(float64(o.rejected))
		}
	}
}

func (m *exportMetrics) sample() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.disabled {
		return
	}
	if drops := m.pendingDrops.Swap(0); drops > 0 {
		m.dropped.Add(float64(drops))
	}
	length := m.queueLength()
	// A meter can be shared by multiple tracers.
	m.length.Add(float64(length - m.lastLength))
	m.lastLength = length
}
