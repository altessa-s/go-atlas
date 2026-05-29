// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/altessa-s/go-atlas/observability/appstats"
)

// BenchmarkStatsLogger_RunLogCycle measures the cost of a full
// statistics-collection + log emission round-trip. Useful for catching
// regressions when GetApplicationStats grows new collectors or the
// downstream slog handler changes.
func BenchmarkStatsLogger_RunLogCycle(b *testing.B) {
	logger := slog.New(slog.DiscardHandler)
	s := appstats.NewStatsLogger(appstats.WithLogger(logger))

	ctx := context.Background()

	for b.Loop() {
		_ = s.RunLogCycle(ctx)
	}
}
