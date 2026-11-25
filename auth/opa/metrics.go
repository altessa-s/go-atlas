// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// labelResult is the metric label holding an evaluation, reload or cache lookup outcome.
const labelResult = "result"

// DefaultMetricsSubsystem is the Prometheus subsystem prefix applied to every
// metric this package emits. It follows the auth_<package> scheme shared with
// the sibling auth/* packages (auth_static, auth_selfjwt, auth_oidc).
const DefaultMetricsSubsystem = "auth_opa"

// opaMetrics holds all Prometheus metrics for the OPA policy manager.
// When no [metrics.Meter] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type opaMetrics struct {
	policyReloads        metrics.Counter
	reloadDuration       metrics.Timer
	evaluations          metrics.Counter
	evaluationDuration   metrics.Timer
	modulesLoaded        metrics.Gauge
	decisionCacheLookups metrics.Counter

	// Pre-bound children of the fixed-label counters above, so the
	// evaluation path does not rebind a label set on every call.
	evalAllow   metrics.Counter
	evalDeny    metrics.Counter
	evalError   metrics.Counter
	cacheHits   metrics.Counter
	cacheMisses metrics.Counter
}

func newOpaMetrics(c metrics.Meter) *opaMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := metrics.MustMeter{Meter: c.WithSubsystem(DefaultMetricsSubsystem)}

	m := &opaMetrics{
		policyReloads: scoped.MustCounter(metrics.MetricOpts{
			Name:       "policy_reloads_total",
			Help:       "Total number of policy reload operations.",
			LabelNames: []string{labelResult},
		}),
		reloadDuration: scoped.MustTimer(metrics.HistogramOpts{
			Name: "policy_reload_duration_seconds",
			Help: "Duration of policy reload operations in seconds.",
		}),
		evaluations: scoped.MustCounter(metrics.MetricOpts{
			Name:       "evaluations_total",
			Help:       "Total number of policy evaluations.",
			LabelNames: []string{labelResult},
		}),
		evaluationDuration: scoped.MustTimer(metrics.HistogramOpts{
			Name: "evaluation_duration_seconds",
			Help: "Duration of policy evaluations in seconds.",
		}),
		modulesLoaded: scoped.MustGauge(metrics.MetricOpts{
			Name: "modules_loaded",
			Help: "Number of policy modules currently loaded.",
		}),
		// Separate from evaluations_total, whose "result" label describes the
		// decision (allow/deny); conflating the two would make neither
		// answerable on its own.
		decisionCacheLookups: scoped.MustCounter(metrics.MetricOpts{
			Name:       "decision_cache_lookups_total",
			Help:       "Total number of decision cache lookups, by outcome.",
			LabelNames: []string{labelResult},
		}),
	}

	m.evalAllow = m.evaluations.WithLabels(metrics.Labels{labelResult: "allow"})
	m.evalDeny = m.evaluations.WithLabels(metrics.Labels{labelResult: "deny"})
	m.evalError = m.evaluations.WithLabels(metrics.Labels{labelResult: "error"})
	m.cacheHits = m.decisionCacheLookups.WithLabels(metrics.Labels{labelResult: "hit"})
	m.cacheMisses = m.decisionCacheLookups.WithLabels(metrics.Labels{labelResult: "miss"})

	return m
}
