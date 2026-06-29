// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

func TestOpaMetrics_Noop(t *testing.T) {
	m := newOpaMetrics(nil)

	// Should not panic with noop metrics
	m.policyReloads.WithLabels(metrics.Labels{"result": "success"}).Inc()
	stop := m.reloadDuration.Start()
	stop()
	m.evaluations.WithLabels(metrics.Labels{"result": "allow"}).Inc()
	stop = m.evaluationDuration.Start()
	stop()
	m.modulesLoaded.Set(5)
}

func TestOpaMetrics_Prometheus(t *testing.T) {
	tc := testhelpers.NewTestCollector()

	m := newOpaMetrics(tc)

	m.policyReloads.WithLabels(metrics.Labels{"result": "success"}).Inc()
	m.policyReloads.WithLabels(metrics.Labels{"result": "unchanged"}).Inc()
	stop := m.reloadDuration.Start()
	stop()
	m.evaluations.WithLabels(metrics.Labels{"result": "allow"}).Inc()
	m.evaluations.WithLabels(metrics.Labels{"result": "deny"}).Inc()
	stop = m.evaluationDuration.Start()
	stop()
	m.modulesLoaded.Set(3)

	val := testhelpers.GetCounterValue(t, tc, "test_auth_opa_policy_reloads_total", "result", "success")
	assert.Equal(t, float64(1), val)

	val = testhelpers.GetCounterValue(t, tc, "test_auth_opa_policy_reloads_total", "result", "unchanged")
	assert.Equal(t, float64(1), val)

	count := testhelpers.GetHistogramCount(t, tc, "test_auth_opa_policy_reload_duration_seconds")
	assert.GreaterOrEqual(t, count, uint64(1))

	val = testhelpers.GetCounterValue(t, tc, "test_auth_opa_evaluations_total", "result", "allow")
	assert.Equal(t, float64(1), val)

	val = testhelpers.GetCounterValue(t, tc, "test_auth_opa_evaluations_total", "result", "deny")
	assert.Equal(t, float64(1), val)

	count = testhelpers.GetHistogramCount(t, tc, "test_auth_opa_evaluation_duration_seconds")
	assert.GreaterOrEqual(t, count, uint64(1))

	gauge := testhelpers.GetGaugeValue(t, tc, "test_auth_opa_modules_loaded")
	assert.Equal(t, float64(3), gauge)
}
