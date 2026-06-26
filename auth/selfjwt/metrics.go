// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

// DefaultMetricsSubsystem is the subsystem used when [NewMetrics] receives an
// empty subsystem name.
const DefaultMetricsSubsystem = "auth_selfjwt"

const (
	statusSuccess = "success"
	statusFailure = "failure"

	cacheResultHit  = "hit"
	cacheResultMiss = "miss"
)

// Metrics collects mint/verify telemetry for [Minter] and [Verifier].
//
// A nil *Metrics is a valid receiver — every recording method becomes a no-op,
// so wiring metrics is entirely optional.
type Metrics struct {
	mints          metrics.Counter
	verifications  metrics.Counter
	mintDuration   metrics.Timer
	verifyDuration metrics.Timer
	cacheLookups   metrics.Counter
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
		mints: scoped.MustCounter(metrics.MetricOpts{
			Name:       "mints_total",
			Help:       "Total number of token mint attempts.",
			LabelNames: []string{"status"},
		}),
		verifications: scoped.MustCounter(metrics.MetricOpts{
			Name:       "verifications_total",
			Help:       "Total number of token verification attempts.",
			LabelNames: []string{"status"},
		}),
		mintDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name:       "mint_duration_seconds",
				Help:       "Token mint duration in seconds.",
				LabelNames: []string{"status"},
			},
		}),
		verifyDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name:       "verify_duration_seconds",
				Help:       "Token verification duration in seconds.",
				LabelNames: []string{"status"},
			},
		}),
		cacheLookups: scoped.MustCounter(metrics.MetricOpts{
			Name:       "verification_key_cache_lookups_total",
			Help:       "Verification-key cache lookups by result.",
			LabelNames: []string{"result"},
		}),
	}
}

// recordMint records a mint attempt together with its duration.
// Safe to call on a nil receiver.
func (m *Metrics) recordMint(success bool, d time.Duration) {
	if m == nil {
		return
	}
	label := metrics.Labels{"status": status(success)}
	m.mints.WithLabels(label).Inc()
	m.mintDuration.WithLabels(label).ObserveDuration(d)
}

// recordVerify records a verification attempt together with its duration.
// Safe to call on a nil receiver.
func (m *Metrics) recordVerify(success bool, d time.Duration) {
	if m == nil {
		return
	}
	label := metrics.Labels{"status": status(success)}
	m.verifications.WithLabels(label).Inc()
	m.verifyDuration.WithLabels(label).ObserveDuration(d)
}

// recordCacheLookup records a verification-key cache hit or miss.
// Safe to call on a nil receiver.
func (m *Metrics) recordCacheLookup(hit bool) {
	if m == nil {
		return
	}
	result := cacheResultMiss
	if hit {
		result = cacheResultHit
	}
	m.cacheLookups.WithLabels(metrics.Labels{"result": result}).Inc()
}

// status maps a success flag to its metric label value.
func status(success bool) string {
	if success {
		return statusSuccess
	}
	return statusFailure
}
