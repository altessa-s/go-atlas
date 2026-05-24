// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static

import (
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

// DefaultMetricsSubsystem is the subsystem used when [NewMetrics] receives an
// empty subsystem name.
const DefaultMetricsSubsystem = "auth_static"

const (
	statusSuccess = "success"
	statusFailure = "failure"
)

// Metrics collects validation telemetry for [InMemoryStore] (and any other
// [TokenStore] that calls [Metrics.RecordValidation]).
//
// A nil *Metrics is a valid receiver — every recording method becomes a no-op.
type Metrics struct {
	validations        metrics.Counter
	activeTokens       metrics.Gauge
	validationDuration metrics.Timer
}

// NewMetrics constructs a [Metrics] using the given collector and subsystem.
// If collector is nil, [metrics.Noop] is used and all metric writes become
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
		validations: scoped.MustCounter(metrics.MetricOpts{
			Name:       "validations_total",
			Help:       "Total number of token validation attempts.",
			LabelNames: []string{"status"},
		}),
		activeTokens: scoped.MustGauge(metrics.MetricOpts{
			Name: "tokens_active",
			Help: "Current number of active tokens in the store.",
		}),
		validationDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "validation_duration_seconds",
				Help: "Token validation duration in seconds.",
			},
		}),
	}
}

// RecordValidation records a validation attempt together with its duration.
// Safe to call on a nil receiver.
func (m *Metrics) RecordValidation(success bool, d time.Duration) {
	if m == nil {
		return
	}
	status := statusFailure
	if success {
		status = statusSuccess
	}
	m.validations.WithLabels(metrics.Labels{"status": status}).Inc()
	m.validationDuration.ObserveDuration(d)
}

// SetActiveTokens publishes the current size of the underlying store.
// Safe to call on a nil receiver.
func (m *Metrics) SetActiveTokens(n int) {
	if m == nil {
		return
	}
	m.activeTokens.Set(float64(n))
}
