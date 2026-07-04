// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mirror_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist/mirror"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

const (
	refreshesMetric = "test_auth_denylist_mirror_refreshes_total"
	snapshotMetric  = "test_auth_denylist_mirror_snapshot_size"
)

func TestMetrics_RecordsRefreshOutcomes(t *testing.T) {
	t.Parallel()
	tc := testhelpers.NewTestCollector()
	s := &swapSource{keys: []string{"a", "b"}}
	m := mirror.New(s, mirror.WithMetrics(mirror.NewMetrics(tc, "")))

	require.NoError(t, m.Refresh(t.Context()))
	require.Equal(t, 1.0, testhelpers.GetCounterValue(t, tc, refreshesMetric, "result", "ok"))
	require.Equal(t, 2.0, testhelpers.GetGaugeValue(t, tc, snapshotMetric))

	// A failed refresh records result=error and leaves snapshot_size untouched.
	s.setErr(errors.New("store down"))
	require.Error(t, m.Refresh(t.Context()))
	require.Equal(t, 1.0, testhelpers.GetCounterValue(t, tc, refreshesMetric, "result", "error"))
	require.Equal(t, 2.0, testhelpers.GetGaugeValue(t, tc, snapshotMetric))
}

func TestMetrics_NilIsNoOp(t *testing.T) {
	t.Parallel()
	// New without WithMetrics leaves the metrics seam nil; recording is a no-op.
	m := mirror.New(sliceSource{"a"})
	require.NoError(t, m.Refresh(t.Context()))
	require.True(t, m.IsRevoked("a"))
}
