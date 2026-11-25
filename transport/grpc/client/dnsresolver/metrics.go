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

const labelResult = "result"

// builderMetrics holds the instruments shared by the resolvers of a builder.
// Without a [metrics.Meter], [metrics.Noop] makes every update free.
type builderMetrics struct {
	ok, empty, failed metrics.Counter
	addresses         metrics.Gauge
}

func newBuilderMetrics(meter metrics.Meter) *builderMetrics {
	if nilcheck.IsNil(meter) {
		meter = metrics.Noop()
	}
	scoped := meter.WithSubsystem(metricsSubsystem)
	lookups := scoped.MustCounter(metrics.MetricOpts{
		Name:       "lookups_total",
		Help:       "gRPC DNS resolution attempts by result: ok, empty or error.",
		LabelNames: []string{labelResult},
	})
	return &builderMetrics{
		ok:     lookups.WithLabels(metrics.Labels{labelResult: "ok"}),
		empty:  lookups.WithLabels(metrics.Labels{labelResult: "empty"}),
		failed: lookups.WithLabels(metrics.Labels{labelResult: "error"}),
		addresses: scoped.MustGauge(metrics.MetricOpts{
			Name: "addresses",
			Help: "Total addresses last submitted by active gRPC DNS resolvers.",
		}),
	}
}
