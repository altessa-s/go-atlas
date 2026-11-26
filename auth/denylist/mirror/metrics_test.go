// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mirror_test

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist/mirror"
	"github.com/altessa-s/go-atlas/auth/jwt"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
)

const (
	refreshesMetric   = "test_auth_denylist_mirror_refreshes_total"
	snapshotMetric    = "test_auth_denylist_mirror_snapshot_size"
	unavailableMetric = "test_auth_denylist_mirror_unavailable_total"
)

func TestMetrics_RecordsRefreshOutcomes(t *testing.T) {
	t.Parallel()
	tc := metricstest.NewTestCollector()
	s := &swapSource{keys: []string{"a", "b"}}
	m := mirror.New(s, mirror.WithMetrics(mirror.NewMetrics(tc, "")))

	require.NoError(t, m.Refresh(t.Context()))
	require.Equal(t, 1.0, metricstest.GetCounterValue(t, tc, refreshesMetric, "result", "ok"))
	require.Equal(t, 2.0, metricstest.GetGaugeValue(t, tc, snapshotMetric))

	// A failed refresh records result=error and leaves snapshot_size untouched.
	s.setErr(errors.New("store down"))
	require.Error(t, m.Refresh(t.Context()))
	require.Equal(t, 1.0, metricstest.GetCounterValue(t, tc, refreshesMetric, "result", "error"))
	require.Equal(t, 2.0, metricstest.GetGaugeValue(t, tc, snapshotMetric))
}

func TestMetrics_NilIsNoOp(t *testing.T) {
	t.Parallel()
	// New without WithMetrics leaves the metrics seam nil; recording is a no-op.
	m := mirror.New(sliceSource{"a"})
	require.NoError(t, m.Refresh(t.Context()))
	require.True(t, isRevoked(t, m, "a"))
}

func TestMetrics_RecordsRefusedLookups(t *testing.T) {
	t.Parallel()
	tc := metricstest.NewTestCollector()
	m := mirror.New(&swapSource{}, mirror.WithMetrics(mirror.NewMetrics(tc, "")))

	for range 3 {
		_, err := m.IsRevoked("a")
		require.ErrorIs(t, err, jwt.ErrRevocationUnavailable)
	}
	require.Equal(t, 3.0, metricstest.GetCounterValue(t, tc, unavailableMetric, "reason", "unprimed"))
}

func TestLogsOncePerStateChange(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	src := &swapSource{}
	m := mirror.New(src, mirror.WithLogger(logger))

	for range 5 {
		_, _ = m.IsRevoked("a")
	}
	require.Equal(t, 1, strings.Count(buf.String(), "refusing revocation checks"))

	require.NoError(t, m.Prime(t.Context()))
	_, err := m.IsRevoked("a")
	require.NoError(t, err)
	require.Contains(t, buf.String(), "serving revocation checks again")
}
