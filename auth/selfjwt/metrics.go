// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

// Metric label names.
const (
	labelStatus = "status"
	labelResult = "result"
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
func NewMetrics(collector metrics.Meter, subsystem string) *Metrics {
	if collector == nil {
		collector = metrics.Noop()
	}
	if subsystem == "" {
		subsystem = DefaultMetricsSubsystem
	}
	scoped := metrics.MustMeter{Meter: collector.WithSubsystem(subsystem)}

	return &Metrics{
		mints: scoped.MustCounter(metrics.MetricOpts{
			Name:       "mints_total",
			Help:       "Total number of token mint attempts.",
			LabelNames: []string{labelStatus},
		}),
		verifications: scoped.MustCounter(metrics.MetricOpts{
			Name:       "verifications_total",
			Help:       "Total number of token verification attempts.",
			LabelNames: []string{labelStatus},
		}),
		mintDuration: scoped.MustTimer(metrics.HistogramOpts{
			Name:       "mint_duration_seconds",
			Help:       "Token mint duration in seconds.",
			LabelNames: []string{labelStatus},
		}),
		verifyDuration: scoped.MustTimer(metrics.HistogramOpts{
			Name:       "verify_duration_seconds",
			Help:       "Token verification duration in seconds.",
			LabelNames: []string{labelStatus},
		}),
		cacheLookups: scoped.MustCounter(metrics.MetricOpts{
			Name:       "verification_key_cache_lookups_total",
			Help:       "Verification-key cache lookups by result.",
			LabelNames: []string{labelResult},
		}),
	}
}

// recordMint records a mint attempt together with its duration.
// Safe to call on a nil receiver.
func (m *Metrics) recordMint(success bool, d time.Duration) {
	if m == nil {
		return
	}
	label := metrics.Labels{labelStatus: status(success)}
	m.mints.WithLabels(label).Inc()
	m.mintDuration.WithLabels(label).ObserveDuration(d)
}

// recordVerify records a verification attempt together with its duration.
// Safe to call on a nil receiver.
func (m *Metrics) recordVerify(success bool, d time.Duration) {
	if m == nil {
		return
	}
	label := metrics.Labels{labelStatus: status(success)}
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
	m.cacheLookups.WithLabels(metrics.Labels{labelResult: result}).Inc()
}

// status maps a success flag to its metric label value.
func status(success bool) string {
	if success {
		return statusSuccess
	}
	return statusFailure
}
