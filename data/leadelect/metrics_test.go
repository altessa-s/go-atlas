// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

func TestLeaderMetrics_Noop(t *testing.T) {
	m := newLeaderMetrics(nil)

	// Should not panic with noop metrics
	m.transitions.WithLabels(metrics.Labels{"type": "became_leader"}).Inc()
	m.isLeader.Set(1)
	stop := m.callbackDuration.Start()
	stop()
	m.callbackErrors.Inc()
}

func TestLeaderMetrics_Prometheus(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := testhelpers.NewTestCollector(registry)
	m := newLeaderMetrics(collector)

	m.transitions.WithLabels(metrics.Labels{"type": "became_leader"}).Inc()
	m.isLeader.Set(1)
	stop := m.callbackDuration.Start()
	stop()
	m.callbackErrors.Inc()

	val := testhelpers.GetCounterValue(t, registry, "test_leader_election_transitions_total",
		"type", "became_leader")
	assert.Equal(t, float64(1), val)

	gauge := testhelpers.GetGaugeValue(t, registry, "test_leader_election_is_leader")
	assert.Equal(t, float64(1), gauge)

	count := testhelpers.GetHistogramCount(t, registry, "test_leader_election_callback_duration_seconds")
	assert.GreaterOrEqual(t, count, uint64(1))

	errors := testhelpers.GetCounterValue(t, registry, "test_leader_election_callback_errors_total")
	assert.Equal(t, float64(1), errors)
}
