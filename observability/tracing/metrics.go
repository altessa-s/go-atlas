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
)

type exportMetrics struct {
	exported, dropped, failures metrics.Counter
	length                      metrics.Gauge
	queueLength                 func() int
	pendingDrops                atomic.Uint64
	mu                          sync.Mutex
	disabled                    bool // protected by mu
	lastLength                  int
	done                        chan struct{}
}

func newExportMetrics(ctx context.Context, meter metrics.Meter, queueLength func() int) *exportMetrics {
	if meter == nil || metrics.IsNoop(meter) {
		return nil
	}
	m := metrics.MustMeter{Meter: meter.WithSubsystem("tracing")}
	stats := &exportMetrics{
		exported:    m.MustCounter(metrics.MetricOpts{Name: "spans_exported_total", Help: "Spans in successful batch exports."}),
		dropped:     m.MustCounter(metrics.MetricOpts{Name: "spans_dropped_total", Help: "Sampled spans dropped because the export queue was full."}),
		failures:    m.MustCounter(metrics.MetricOpts{Name: "export_failures_total", Help: "Failed batch export calls."}),
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
				stats.sample(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
	return stats
}

// disable stops both metric writers and removes this tracer's queue gauge contribution.
func (m *exportMetrics) disable() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.disabled = true
	if m.lastLength != 0 {
		m.length.Add(-float64(m.lastLength))
		m.lastLength = 0
	}
}

func (m *exportMetrics) recordExport(ctx context.Context, count int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.disabled || ctx.Err() != nil {
		return
	}
	if err != nil {
		m.failures.Inc()
	} else {
		m.exported.Add(float64(count))
	}
}

func (m *exportMetrics) sample(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.disabled || ctx.Err() != nil {
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
