// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
)

// The Find paths reach operation only against a live server; pin its label
// binding and per-(op, collection) caching directly.
func TestMongoMetrics_OperationCachesLabelBoundHandles(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	m := newMongoMetrics(tc)

	find := m.operation("find", "users")
	require.Same(t, find, m.operation("find", "users"), "handles are cached per (op, collection)")
	require.NotSame(t, find, m.operation("find_one", "users"))
	require.NotSame(t, find, m.operation("find", "orders"))

	find.total.Inc()
	find.total.Inc()
	m.operation("find_one", "users").total.Inc()

	require.InDelta(t, 2, metricstest.GetCounterValue(t, tc, "test_mongo_operations_total", "op", "find", "collection", "users"), 0.001)
	require.InDelta(t, 1, metricstest.GetCounterValue(t, tc, "test_mongo_operations_total", "op", "find_one", "collection", "users"), 0.001)
	require.Zero(t, metricstest.GetCounterValue(t, tc, "test_mongo_operations_total", "op", "find", "collection", "orders"))
}
