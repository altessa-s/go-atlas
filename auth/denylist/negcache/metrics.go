// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package negcache

import "github.com/altessa-s/go-atlas/observability/metrics"

// DefaultMetricsSubsystem is the subsystem used when [NewMetrics] receives an
// empty subsystem name.
const DefaultMetricsSubsystem = "auth_denylist_negcache"

// Lookup resolution paths, used as the "result" label value.
const (
	resultFastNegative       = "fast_negative"       // filter miss: answered locally, no authoritative call.
	resultAuthoritativeHit   = "authoritative_hit"   // fell through: authoritative store reports revoked.
	resultAuthoritativeMiss  = "authoritative_miss"  // fell through: authoritative store reports not revoked.
	resultAuthoritativeError = "authoritative_error" // fell through: authoritative store returned an error.
)

// Negative-filter state, used as the "filter" label value.
const (
	filterOK    = "ok"    // the filter answered.
	filterError = "error" // the filter errored and the lookup fell back to the authoritative store.
)

// Metrics collects lookup telemetry for a [Cache].
//
// A nil *Metrics is a valid receiver — every recording method becomes a no-op,
// so wiring metrics is entirely optional.
type Metrics struct {
	lookups metrics.Counter
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
		lookups: scoped.MustCounter(metrics.MetricOpts{
			Name:       "lookups_total",
			Help:       "Revocation lookups by resolution path and negative-filter state.",
			LabelNames: []string{"result", "filter"},
		}),
	}
}

// recordLookup records one lookup by its resolution path and the filter state
// that produced it. Safe to call on a nil receiver. The label pair also yields
// the cache hit rate: fast_negative lookups are the ones that skipped the
// authoritative round trip.
func (m *Metrics) recordLookup(result, filter string) {
	if m == nil {
		return
	}
	m.lookups.WithLabels(metrics.Labels{"result": result, "filter": filter}).Inc()
}
