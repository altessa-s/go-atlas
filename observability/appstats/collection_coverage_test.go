// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats_test

import (
	"bytes"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/appstats"
)

// The background collector is process-wide state shared with the other tests
// of this package, so this test runs serially. The cache timestamps outlive a
// stopped collection, so they are reset first: otherwise a rerun in the same
// process (-count) finds the cache fresh before the first tick.
func TestBackgroundCollection_PopulatesCache(t *testing.T) {
	appstats.ResetMetricsCollection()
	appstats.StartMetricsCollectionWithContext(t.Context())
	t.Cleanup(appstats.StopMetricsCollection)

	// The first tick fires after DefaultCacheInterval; it refreshes every
	// metric family, so the cache stops being stale.
	testhelpers.WaitFor(t, 5*appstats.DefaultCacheInterval, func() bool {
		return !appstats.IsCacheStale()
	}, "background collection never refreshed the cache")

	cpuLoad, err := appstats.CPULoad(t.Context())
	require.NoError(t, err)
	requirePercent(t, cpuLoad.Service)
	requirePercent(t, cpuLoad.System)
	require.Equal(t, cpuLoad.System, cpuLoad.Total)

	memLoad, err := appstats.MemoryLoad(t.Context())
	require.NoError(t, err)
	requirePercent(t, memLoad.Service)
	requirePercent(t, memLoad.System)
	total, err := strconv.ParseFloat(memLoad.Total, 64)
	require.NoError(t, err)
	require.Positive(t, total, "total system memory is collected")

	stats := appstats.GetApplicationStats(t.Context())
	require.Equal(t, memLoad.Total, stats.Memory.Total)
	require.True(t, strings.HasSuffix(stats.Network.BytesReceived, " bytes"), stats.Network.BytesReceived)
	require.True(t, strings.HasSuffix(stats.Network.BytesSent, " bytes"), stats.Network.BytesSent)
}

func TestNewStatsLogger_RunLogCycleWritesStats(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	s := appstats.NewStatsLogger(appstats.WithLogger(nil), appstats.WithLogger(logger))
	t.Cleanup(appstats.StopMetricsCollection)

	require.NoError(t, s.RunLogCycle(t.Context()))
	out := buf.String()
	require.Contains(t, out, `"process"`)
	require.Contains(t, out, `"memory"`)
}

func requirePercent(t *testing.T, s string) {
	t.Helper()
	require.True(t, strings.HasSuffix(s, "%"), "want a percentage, got %q", s)
	v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
	require.NoError(t, err)
	require.GreaterOrEqual(t, v, 0.0)
}
