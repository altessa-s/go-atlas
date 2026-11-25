// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

//go:generate go tool optgen generate --type=options

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"

	schedtask "github.com/altessa-s/go-horai/task"
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
	// watchChannelSize sets the buffer size for watch event channels
	// created by Watch when WatchOptions.BufferSize is zero.
	// Defaults to 10.
	watchChannelSize int `optgen:"default=DefaultWatchBufferSize"`
	// pollInterval is the interval between polling the source for changes.
	// Defaults to 30s.
	pollInterval time.Duration `optgen:"default=DefaultPollInterval"`
	// healthCoordinator registers the manager with a health coordinator.
	healthCoordinator health.Registrar `optgen:"notnil"`
	// WithSchedulerTaskID sets the policy update task ID. An empty ID uses
	// "opa-policy-update-" plus a stable digest of the source's Name() and the Rego query.
	// Managers with the same name and query sharing one registrar must set distinct
	// IDs explicitly if each needs its own task.
	schedulerTaskID string
	// scheduler sets the scheduler for automatic policy updates.
	scheduler schedtask.Registrar `optgen:"notnil"`
	// Set by WithUpdateSchedule: schedule and runOnStart are assigned together.
	updateSchedule string `optgen:"manual"`
	// Set by WithUpdateSchedule: schedule and runOnStart are assigned together.
	runOnStart bool `optgen:"manual"`
	// meter for metrics collection.
	meter metrics.Meter `optgen:"notnil"`
	// auditRecorder records authorization decisions for every evaluation.
	// Nil by default, which disables auditing.
	auditRecorder DecisionRecorder `optgen:"notnil"`
	//optgen:doc DecisionCache WithDecisionCache memoizes deterministic evaluations for ttl, keeping at most size entries per manager.
	//optgen:doc DecisionCacheBackend WithDecisionCacheBackend supplies a caller-owned decision cache. A nil cache disables caching.
	decisionCache DecisionCache `optfn:"DecisionCache=newDecisionCache, DecisionCacheBackend=decisionCacheBackend"`
}

// WithUpdateSchedule configures periodic policy updates. A nonempty schedule requires WithScheduler.
// The schedule parameter should be a cron expression (e.g., "0 */5 * * * *" for every 5 minutes).
// Requires WithScheduler to take effect.
func WithUpdateSchedule(schedule string, runOnStart bool) Option {
	return func(o *options) {
		o.updateSchedule = schedule
		o.runOnStart = runOnStart
	}
}
