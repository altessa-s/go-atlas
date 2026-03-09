// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

func TestVaultAuthMetrics_Noop(t *testing.T) {
	m := newVaultAuthMetrics(nil)

	// Should not panic with noop metrics
	m.authAttempts.Inc()
	m.authErrors.WithLabels(metrics.Labels{"type": "permanent"}).Inc()
	m.tokenRenewals.Inc()
	m.tokenRenewalErrors.Inc()
}

func TestVaultAuthMetrics_Prometheus(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := testhelpers.NewTestCollector(registry)
	m := newVaultAuthMetrics(collector)

	m.authAttempts.Inc()
	m.authAttempts.Inc()
	m.authErrors.WithLabels(metrics.Labels{"type": "permanent"}).Inc()
	m.authErrors.WithLabels(metrics.Labels{"type": "transient"}).Inc()
	m.tokenRenewals.Inc()
	m.tokenRenewalErrors.Inc()

	val := testhelpers.GetCounterValue(t, registry, "test_vault_auth_auth_attempts_total")
	assert.Equal(t, float64(2), val)

	val = testhelpers.GetCounterValue(t, registry, "test_vault_auth_auth_errors_total", "type", "permanent")
	assert.Equal(t, float64(1), val)

	val = testhelpers.GetCounterValue(t, registry, "test_vault_auth_auth_errors_total", "type", "transient")
	assert.Equal(t, float64(1), val)

	val = testhelpers.GetCounterValue(t, registry, "test_vault_auth_token_renewals_total")
	assert.Equal(t, float64(1), val)

	val = testhelpers.GetCounterValue(t, registry, "test_vault_auth_token_renewal_errors_total")
	assert.Equal(t, float64(1), val)
}
