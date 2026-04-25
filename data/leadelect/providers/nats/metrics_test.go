// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

func TestNatsLeaderMetrics_Noop(t *testing.T) {
	m := newNatsLeaderMetrics(nil)

	// Should not panic with noop metrics
	m.leaseOperations.WithLabels(metrics.Labels{"op": "acquire", "result": "success"}).Inc()
	m.isLeader.Set(1)
	stop := m.campingIterations.Start()
	stop()
}

func TestNatsLeaderMetrics_Prometheus(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := testhelpers.NewTestCollector(registry)
	m := newNatsLeaderMetrics(collector)

	m.leaseOperations.WithLabels(metrics.Labels{"op": "acquire", "result": "success"}).Inc()
	m.leaseOperations.WithLabels(metrics.Labels{"op": "renew", "result": "failure"}).Inc()
	m.isLeader.Set(1)
	stop := m.campingIterations.Start()
	stop()

	val := testhelpers.GetCounterValue(t, registry, "test_nats_leader_election_lease_operations_total",
		"op", "acquire", "result", "success")
	assert.Equal(t, float64(1), val)

	val = testhelpers.GetCounterValue(t, registry, "test_nats_leader_election_lease_operations_total",
		"op", "renew", "result", "failure")
	assert.Equal(t, float64(1), val)

	gauge := testhelpers.GetGaugeValue(t, registry, "test_nats_leader_election_is_leader")
	assert.Equal(t, float64(1), gauge)

	count := testhelpers.GetHistogramCount(t, registry, "test_nats_leader_election_camping_iteration_duration_seconds")
	assert.GreaterOrEqual(t, count, uint64(1))
}
