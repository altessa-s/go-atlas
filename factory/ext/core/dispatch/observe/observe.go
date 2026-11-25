// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observe

import (
	"cmp"
	"time"

	"github.com/altessa-s/go-atlas-core/runtime/dispatch"
	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// Observer records dispatch engine events as metrics. Create it with [New].
type Observer struct {
	enqueued      metrics.Counter
	dropped       metrics.Counter
	flushDuration metrics.Timer
	sinkErrors    metrics.Counter
	walErrors     metrics.Counter
	encodeErrors  metrics.Counter
	workersActive metrics.Gauge
	walBytes      metrics.Gauge
	replayCount   metrics.Counter
}

var _ dispatch.Observer = (*Observer)(nil)

// New registers the engine metrics on meter and returns an Observer that
// updates them. A nil meter yields an Observer whose instruments are no-ops.
func New(meter metrics.Meter, opts ...Option) *Observer {
	o := newOptions(opts...)
	if nilcheck.IsNil(meter) {
		meter = metrics.Noop()
	}
	scoped := metrics.MustMeter{Meter: meter.WithSubsystem(cmp.Or(o.metricsSubsystem, DefaultMetricsSubsystem))}
	return &Observer{
		enqueued: scoped.MustCounter(metrics.MetricOpts{
			Name: "items_enqueued_total",
			Help: "Total items successfully enqueued for async dispatch.",
		}),
		dropped: scoped.MustCounter(metrics.MetricOpts{
			Name: "items_dropped_total",
			Help: "Total items dropped on admission.",
		}),
		flushDuration: scoped.MustTimer(metrics.HistogramOpts{
			Name: "batch_flush_duration_seconds",
			Help: "Duration of batch sink store operations, retries included.",
		}),
		sinkErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "sink_errors_total",
			Help: "Total items of failed sink attempts, retries included.",
		}),
		walErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "wal_errors_total",
			Help: "Total WAL append errors.",
		}),
		encodeErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "encode_errors_total",
			Help: "Total codec encode failures.",
		}),
		workersActive: scoped.MustGauge(metrics.MetricOpts{
			Name: "workers_active",
			Help: "Number of currently active dispatch workers.",
		}),
		walBytes: scoped.MustGauge(metrics.MetricOpts{
			Name: "wal_bytes",
			Help: "Total bytes currently in WAL across all segments.",
		}),
		replayCount: scoped.MustCounter(metrics.MetricOpts{
			Name: "wal_replay_total",
			Help: "Total records replayed from WAL on startup.",
		}),
	}
}

// ItemEnqueued implements [dispatch.Observer].
func (o *Observer) ItemEnqueued() { o.enqueued.Inc() }

// ItemDropped implements [dispatch.Observer].
func (o *Observer) ItemDropped() { o.dropped.Inc() }

// ItemsReplayed implements [dispatch.Observer].
func (o *Observer) ItemsReplayed(n int) { o.replayCount.Add(float64(n)) }

// EncodeFailed implements [dispatch.Observer].
func (o *Observer) EncodeFailed() { o.encodeErrors.Inc() }

// WALAppendFailed implements [dispatch.Observer].
func (o *Observer) WALAppendFailed() { o.walErrors.Inc() }

// BatchStoreFailed implements [dispatch.Observer].
func (o *Observer) BatchStoreFailed(items int) { o.sinkErrors.Add(float64(items)) }

// BatchDone implements [dispatch.Observer].
func (o *Observer) BatchDone(d time.Duration) { o.flushDuration.ObserveDuration(d) }

// WorkerStarted implements [dispatch.Observer].
func (o *Observer) WorkerStarted() { o.workersActive.Inc() }

// WorkerStopped implements [dispatch.Observer].
func (o *Observer) WorkerStopped() { o.workersActive.Dec() }

// WALSize implements [dispatch.Observer].
func (o *Observer) WALSize(bytes int64) { o.walBytes.Set(float64(bytes)) }
