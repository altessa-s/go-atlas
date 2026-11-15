// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observe

//go:generate go tool optgen generate --type=options

// DefaultMetricsSubsystem is the metrics subsystem used when none is set with
// [WithMetricsSubsystem]. It stays "async" so dashboards and alerts built
// against earlier locations of the dispatch engine keep matching.
const DefaultMetricsSubsystem = "async"

// options holds the tunables of an [Observer].
type options struct {
	metricsSubsystem string `optgen:"default=DefaultMetricsSubsystem"`
}
