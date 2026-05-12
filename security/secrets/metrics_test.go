// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestSecretsMetrics_Noop(t *testing.T) {
	m := newSecretsMetrics(nil)

	// Should not panic with noop metrics
	m.cacheHits.Inc()
	m.cacheMisses.Inc()
	stop := m.fetchDuration.Start()
	stop()
	stop = m.updateCycleDuration.Start()
	stop()
	m.updateCycleErrors.Inc()
	m.cacheSize.Set(10)
}

func TestSecretsMetrics_Prometheus(t *testing.T) {
	tc := testhelpers.NewTestCollector()

	m := newSecretsMetrics(tc)

	m.cacheHits.Inc()
	m.cacheHits.Inc()
	m.cacheMisses.Inc()
	stop := m.fetchDuration.Start()
	stop()
	stop = m.updateCycleDuration.Start()
	stop()
	m.updateCycleErrors.Inc()
	m.cacheSize.Set(42)

	val := testhelpers.GetCounterValue(t, tc, "test_secrets_cache_hits_total")
	assert.Equal(t, float64(2), val)

	val = testhelpers.GetCounterValue(t, tc, "test_secrets_cache_misses_total")
	assert.Equal(t, float64(1), val)

	count := testhelpers.GetHistogramCount(t, tc, "test_secrets_fetch_duration_seconds")
	assert.GreaterOrEqual(t, count, uint64(1))

	count = testhelpers.GetHistogramCount(t, tc, "test_secrets_update_cycle_duration_seconds")
	assert.GreaterOrEqual(t, count, uint64(1))

	val = testhelpers.GetCounterValue(t, tc, "test_secrets_update_cycle_errors_total")
	assert.Equal(t, float64(1), val)

	gauge := testhelpers.GetGaugeValue(t, tc, "test_secrets_cache_size")
	assert.Equal(t, float64(42), gauge)
}
