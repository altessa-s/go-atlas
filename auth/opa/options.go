// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/observability/health"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// DefaultPollInterval is the default interval between polling cycles
// when the Manager is watching for policy changes.
const DefaultPollInterval = 30 * time.Second

// options holds the configuration options for the Manager.
type options struct {
	// logger sets the logger for the Manager.
	logger *slog.Logger
	// decisionLogging enables logging of all policy evaluation decisions.
	decisionLogging bool
	// watchChannelSize sets the buffer size for watch event channels.
	// Defaults to 10.
	watchChannelSize int `optgen:"default=DefaultWatchBufferSize"`
	// pollInterval is the interval between polling the source for changes.
	// Defaults to 30s.
	pollInterval time.Duration `optgen:"default=DefaultPollInterval"`
	// healthCoordinator registers the manager with a health coordinator.
	healthCoordinator *health.Coordinator
	// scheduler sets the scheduler for automatic policy updates.
	scheduler corescheduler.TaskRegistrar `optgen:"notnil"`
	// updateSchedule is a cron expression for periodic policy updates.
	updateSchedule string `opt:"-"`
	// runOnStart triggers an immediate update cycle when starting.
	runOnStart bool `opt:"-"`
}

// WithUpdateSchedule configures periodic policy update task for the scheduler.
// The schedule parameter should be a cron expression (e.g., "0 */5 * * * *" for every 5 minutes).
// Requires WithScheduler to take effect.
func WithUpdateSchedule(schedule string, runOnStart bool) Option {
	return func(o *options) {
		o.updateSchedule = schedule
		o.runOnStart = runOnStart
	}
}
