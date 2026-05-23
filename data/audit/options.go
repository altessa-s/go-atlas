// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

// options holds auditor-specific tunables. Engine-level configuration
// (buffer, batch, workers, WAL, retries) is NOT here — it belongs on
// the [Dispatcher] implementation that the caller passes to [New].
type options struct {
	serviceInfo      ServiceInfo
	logger           *slog.Logger
	collector        metrics.Collector
	metricsSubsystem string `optgen:"default=DefaultMetricsSubsystem"`
}
