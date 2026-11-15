// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/messaging/broker"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
)

// TestCreateInProgressManager_MapsTheMetricsBlock pins that
// inProgress.metrics.enabled decides whether the manager records its metrics
// on the builder's meter.
func TestCreateInProgressManager_MapsTheMetricsBlock(t *testing.T) {
	t.Parallel()

	const metric = "test_broker_inprogress_heartbeats_sent_total"

	tests := []struct {
		name    string
		enabled bool
	}{
		{name: "enabled registers on the meter", enabled: true},
		{name: "disabled leaves the meter untouched", enabled: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := brokerconfig.Default()
			cfg.InProgress.Metrics.Enabled = tc.enabled
			col := metricstest.NewTestCollector()

			mgr, err := factory.New(&cfg).UseMeter(col).CreateInProgressManager()
			require.NoError(t, err)
			require.NotNil(t, mgr)
			require.Equal(t, tc.enabled, metricstest.GatherMetric(t, col, metric))
		})
	}

	t.Run("enabled without a meter builds a manager", func(t *testing.T) {
		t.Parallel()

		cfg := brokerconfig.Default()
		mgr, err := factory.New(&cfg).CreateInProgressManager()
		require.NoError(t, err)
		require.NotNil(t, mgr)
	})
}
