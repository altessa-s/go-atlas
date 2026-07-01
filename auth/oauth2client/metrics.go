// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

// DefaultMetricsSubsystem is the subsystem used when [NewMetrics] receives an
// empty subsystem name.
const DefaultMetricsSubsystem = "auth_oauth2client"

// Grant label values distinguishing the flow that produced a token.
const (
	grantClientCredentials = "client_credentials"
	grantRefresh           = "refresh_token"
	grantAuthorizationCode = "authorization_code"
	grantTokenExchange     = "token_exchange"
	grantDeviceCode        = "device_code"
)

const (
	statusSuccess = "success"
	statusError   = "error"
)

// Metrics collects token-acquisition telemetry across the client flows: fetch
// counts and latency (labeled by grant and outcome) and retry counts.
//
// A nil *Metrics is a valid receiver — every recording method is a no-op, so
// wiring metrics is entirely optional.
type Metrics struct {
	fetches       metrics.Counter
	fetchDuration metrics.Timer
	retries       metrics.Counter
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
		fetches: scoped.MustCounter(metrics.MetricOpts{
			Name:       "token_fetches_total",
			Help:       "Total number of token acquisitions from the identity provider.",
			LabelNames: []string{"grant", "status"},
		}),
		fetchDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name:       "token_fetch_duration_seconds",
				Help:       "Token acquisition duration in seconds.",
				LabelNames: []string{"grant", "status"},
			},
		}),
		retries: scoped.MustCounter(metrics.MetricOpts{
			Name:       "token_fetch_retries_total",
			Help:       "Total number of token-fetch retries after a transient failure.",
			LabelNames: []string{"grant"},
		}),
	}
}

// recordFetch records a token acquisition together with its duration.
// Safe to call on a nil receiver.
func (m *Metrics) recordFetch(grant string, success bool, d time.Duration) {
	if m == nil {
		return
	}
	label := metrics.Labels{"grant": grant, "status": statusLabel(success)}
	m.fetches.WithLabels(label).Inc()
	m.fetchDuration.WithLabels(label).ObserveDuration(d)
}

// recordRetry records a single retry of a token fetch.
// Safe to call on a nil receiver.
func (m *Metrics) recordRetry(grant string) {
	if m == nil {
		return
	}
	m.retries.WithLabels(metrics.Labels{"grant": grant}).Inc()
}

// statusLabel maps a success flag to its metric label value.
func statusLabel(success bool) string {
	if success {
		return statusSuccess
	}
	return statusError
}
