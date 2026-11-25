// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dnsresolver

import (
	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// metricsSubsystem is the subsystem of the resolver metrics.
const metricsSubsystem = "grpc_dns_resolver"

// Metric label names.
const (
	labelTarget = "target"
	labelResult = "result"
)

// lookupResult is the outcome of one lookup, the value of the result label.
type lookupResult string

const (
	// resultOK is a lookup that returned addresses.
	resultOK lookupResult = "ok"
	// resultEmpty is a lookup that found no address (NXDOMAIN or no records).
	resultEmpty lookupResult = "empty"
	// resultError is a lookup that failed or timed out.
	resultError lookupResult = "error"
)

// builderMetrics holds the instruments shared by the resolvers of a builder.
// Without a [metrics.Meter], [metrics.Noop] makes every update free.
type builderMetrics struct {
	lookups   metrics.Counter
	addresses metrics.Gauge
}

func newBuilderMetrics(meter metrics.Meter) *builderMetrics {
	if nilcheck.IsNil(meter) {
		meter = metrics.Noop()
	}
	scoped := meter.WithSubsystem(metricsSubsystem)
	return &builderMetrics{
		lookups: scoped.MustCounter(metrics.MetricOpts{
			Name:       "lookups_total",
			Help:       "DNS lookups of a gRPC target by result: ok, empty or error.",
			LabelNames: []string{labelTarget, labelResult},
		}),
		addresses: scoped.MustGauge(metrics.MetricOpts{
			Name:       "addresses",
			Help:       "Addresses the latest conclusive DNS lookup of a gRPC target returned.",
			LabelNames: []string{labelTarget},
		}),
	}
}

// targetMetrics are the instruments of one target with their labels bound.
type targetMetrics struct {
	ok, empty, failed metrics.Counter
	addresses         metrics.Gauge
}

func (m *builderMetrics) forTarget(target string) targetMetrics {
	lookups := func(result lookupResult) metrics.Counter {
		return m.lookups.WithLabels(metrics.Labels{labelTarget: target, labelResult: string(result)})
	}
	return targetMetrics{
		ok:        lookups(resultOK),
		empty:     lookups(resultEmpty),
		failed:    lookups(resultError),
		addresses: m.addresses.WithLabels(metrics.Labels{labelTarget: target}),
	}
}

// record counts a lookup and, when it was conclusive, sets the address count.
// A failed lookup leaves the count of the previous conclusive one.
func (m targetMetrics) record(result lookupResult, addresses int) {
	switch result {
	case resultOK:
		m.ok.Inc()
		m.addresses.Set(float64(addresses))
	case resultEmpty:
		m.empty.Inc()
		m.addresses.Set(0)
	default:
		m.failed.Inc()
	}
}
