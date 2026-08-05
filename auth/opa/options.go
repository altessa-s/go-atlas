// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/auth/audit"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"

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
	// watchChannelSize sets the buffer size for watch event channels
	// created by Watch when WatchOptions.BufferSize is zero.
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
	// collector for metrics collection.
	collector metrics.Collector `optgen:"notnil"`
	// auditRecorder records authorization decisions for every evaluation.
	// Nil by default, which disables auditing.
	auditRecorder *audit.Recorder
	// decisionCache memoizes evaluations. Nil disables caching, which is the
	// default. Populated only by [WithDecisionCache].
	decisionCache *decisionCache `opt:"-"`
}

// WithDecisionCache memoizes policy evaluations for ttl, keeping at most size
// entries. Caching is off by default.
//
// A cached decision is still recorded with the audit recorder and still counted
// in the evaluation metrics — the cache short-circuits the Rego evaluation, not
// the handling of the decision.
//
// Entries are keyed by policy revision as well as input, so a policy reload
// makes every prior decision unreachable rather than merely stale. A
// non-positive size or ttl falls back to [DefaultDecisionCacheSize] and
// [DefaultDecisionCacheTTL].
//
// Entries are keyed on the JSON encoding of the input, which costs no
// generality: OPA marshals the input to JSON to evaluate it, so an input that
// cannot be keyed was never evaluable.
func WithDecisionCache(size int, ttl time.Duration) Option {
	return func(o *options) {
		o.decisionCache = newDecisionCache(size, ttl)
	}
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
