// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mirror

import "github.com/altessa-s/go-atlas/observability/metrics"

// DefaultMetricsSubsystem is the subsystem used when [NewMetrics] receives an
// empty subsystem name.
const DefaultMetricsSubsystem = "auth_denylist_mirror"

// Refresh outcomes, used as the "result" label value.
const (
	resultOK    = "ok"    // the snapshot was rebuilt and swapped in.
	resultError = "error" // the source errored; the previous snapshot was kept.
)

// Metrics collects refresh telemetry for a [Cache].
//
// A nil *Metrics is a valid receiver — every recording method becomes a no-op,
// so wiring metrics is entirely optional.
type Metrics struct {
	refreshes    metrics.Counter
	snapshotSize metrics.Gauge
}

// NewMetrics constructs a [Metrics] using the given collector and subsystem. If
// collector is nil, [metrics.Noop] is used and all metric writes become
// zero-cost no-ops. An empty subsystem falls back to [DefaultMetricsSubsystem].
func NewMetrics(collector metrics.Collector, subsystem string) *Metrics {
	if collector == nil {
		collector = metrics.Noop()
	}
	if subsystem == "" {
		subsystem = DefaultMetricsSubsystem
	}
	scoped := collector.WithSubsystem(subsystem)

	return &Metrics{
		refreshes: scoped.MustCounter(metrics.MetricOpts{
			Name:       "refreshes_total",
			Help:       "Snapshot refreshes by outcome (ok swaps in a new snapshot, error keeps the previous one).",
			LabelNames: []string{"result"},
		}),
		snapshotSize: scoped.MustGauge(metrics.MetricOpts{
			Name: "snapshot_size",
			Help: "Number of revoked keys in the current snapshot after the last successful refresh.",
		}),
	}
}

// recordRefresh records one refresh outcome. On success it also publishes the
// resulting snapshot size. Safe to call on a nil receiver.
func (m *Metrics) recordRefresh(result string, size int) {
	if m == nil {
		return
	}
	m.refreshes.WithLabels(metrics.Labels{"result": result}).Inc()
	if result == resultOK {
		m.snapshotSize.Set(float64(size))
	}
}
