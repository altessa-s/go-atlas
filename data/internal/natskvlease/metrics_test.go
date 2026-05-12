// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

func TestLeaseMetrics_Noop(t *testing.T) {
	m := newLeaseMetrics(nil)

	// Should not panic with noop metrics
	m.operations.WithLabels(metrics.Labels{"op": "acquire", "result": "success"}).Inc()
	m.leaseHeld.Set(1)
}

func TestLeaseMetrics_Prometheus(t *testing.T) {
	tc := testhelpers.NewTestCollector()

	m := newLeaseMetrics(tc)

	m.operations.WithLabels(metrics.Labels{"op": "acquire", "result": "success"}).Inc()
	m.operations.WithLabels(metrics.Labels{"op": "renew", "result": "success"}).Inc()
	m.operations.WithLabels(metrics.Labels{"op": "release", "result": "success"}).Inc()
	m.leaseHeld.Set(1)

	val := testhelpers.GetCounterValue(t, tc, "test_nats_kv_lease_operations_total",
		"op", "acquire", "result", "success")
	assert.Equal(t, float64(1), val)

	val = testhelpers.GetCounterValue(t, tc, "test_nats_kv_lease_operations_total",
		"op", "renew", "result", "success")
	assert.Equal(t, float64(1), val)

	val = testhelpers.GetCounterValue(t, tc, "test_nats_kv_lease_operations_total",
		"op", "release", "result", "success")
	assert.Equal(t, float64(1), val)

	gauge := testhelpers.GetGaugeValue(t, tc, "test_nats_kv_lease_lease_held")
	assert.Equal(t, float64(1), gauge)
}
