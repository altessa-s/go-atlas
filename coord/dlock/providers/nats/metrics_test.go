// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
)

func TestLeaseMetricsRenewResults(t *testing.T) {
	t.Parallel()
	collector := metricstest.NewTestCollector()
	observer := newLeaseMetrics(collector)
	observer.RecordOperation("renew", "success")
	observer.RecordOperation("renew", "failure")
	observer.RecordOperation("renew", "failure")

	const operations = "test_nats_kv_lease_operations_total"
	require.EqualValues(t, 1, metricstest.GetCounterValue(t, collector, operations, "op", "renew", "result", "success"))
	require.EqualValues(t, 2, metricstest.GetCounterValue(t, collector, operations, "op", "renew", "result", "failure"))
}
