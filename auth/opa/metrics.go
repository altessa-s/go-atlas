// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// opaMetrics holds all Prometheus metrics for the OPA policy manager.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type opaMetrics struct {
	policyReloads      metrics.Counter
	reloadDuration     metrics.Timer
	evaluations        metrics.Counter
	evaluationDuration metrics.Timer
	modulesLoaded      metrics.Gauge
}

func newOpaMetrics(c metrics.Collector) *opaMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("opa")

	return &opaMetrics{
		policyReloads: scoped.MustCounter(metrics.MetricOpts{
			Name:       "policy_reloads_total",
			Help:       "Total number of policy reload operations.",
			LabelNames: []string{"result"},
		}),
		reloadDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "policy_reload_duration_seconds",
				Help: "Duration of policy reload operations in seconds.",
			},
		}),
		evaluations: scoped.MustCounter(metrics.MetricOpts{
			Name:       "evaluations_total",
			Help:       "Total number of policy evaluations.",
			LabelNames: []string{"result"},
		}),
		evaluationDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "evaluation_duration_seconds",
				Help: "Duration of policy evaluations in seconds.",
			},
		}),
		modulesLoaded: scoped.MustGauge(metrics.MetricOpts{
			Name: "modules_loaded",
			Help: "Number of policy modules currently loaded.",
		}),
	}
}
