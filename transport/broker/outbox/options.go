// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/data/outbox"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// Option is a functional option for configuring the broker outbox adapter.
type Option func(o *adapterOptions)

// adapterOptions collects broker-specific options that are converted to generic outbox options.
type adapterOptions struct {
	genericOpts []outbox.Option
}

// convertOptions converts broker adapter options into generic outbox options.
func convertOptions(opts ...Option) []outbox.Option {
	ao := &adapterOptions{}
	for _, opt := range opts {
		opt(ao)
	}
	return ao.genericOpts
}

// WithLogger sets the logger for outbox operations (dispatch, unlock, cleanup).
func WithLogger(v *slog.Logger) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithLogger(v))
	}
}

// WithContext sets the base context used for outbox background operations.
// Canceling this context stops all dispatch, unlock, and cleanup cycles.
func WithContext(v context.Context) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithContext(v))
	}
}

// WithFetchTimeout sets the maximum duration for fetching pending messages from the store.
func WithFetchTimeout(v time.Duration) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithFetchTimeout(v))
	}
}

// WithHandleTimeout sets the maximum duration for publishing a single message to the broker.
func WithHandleTimeout(v time.Duration) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithHandleTimeout(v))
	}
}

// WithUpdateTimeout sets the maximum duration for updating message status in the store
// after a publish attempt (marking as sent, failed, etc.).
func WithUpdateTimeout(v time.Duration) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithUpdateTimeout(v))
	}
}

// WithMessagesBatchSize sets the maximum number of messages fetched and dispatched per cycle.
func WithMessagesBatchSize(v uint32) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithEventsBatchSize(v))
	}
}

// WithRetryMaxAttempts sets the maximum number of publish attempts per message
// before marking it as [outbox.StatusMaxAttemptReached].
func WithRetryMaxAttempts(v uint32) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithRetryMaxAttempts(v))
	}
}

// WithRetryBaseDelay sets the backoff before the second publish attempt.
// Subsequent attempts back off exponentially with jitter.
func WithRetryBaseDelay(v time.Duration) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithRetryBaseDelay(v))
	}
}

// WithRetryMaxDelay caps the backoff between publish attempts.
func WithRetryMaxDelay(v time.Duration) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithRetryMaxDelay(v))
	}
}

// WithMaxLockTime sets how long a message may stay locked by a dispatcher before
// the unlock cycle reclaims it. It must exceed the handle timeout — a lock that
// expires mid-publish lets a second dispatcher pick the same message up, which
// makes duplicate delivery routine rather than exceptional. See
// [outbox.WithMaxLockTime].
func WithMaxLockTime(v time.Duration) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithMaxLockTime(v))
	}
}

// WithMaxPayloadBytes caps the accepted message size, rejecting larger ones at
// Publish time with [outbox.ErrPayloadTooLarge] rather than deep in the store
// driver. The limit applies to the stored envelope (data plus metadata), which
// is slightly larger than the caller's payload. Pass 0 to disable the check.
func WithMaxPayloadBytes(n int) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithMaxPayloadBytes(n))
	}
}

// WithCollector sets the metrics collector for the outbox instrumentation —
// dispatch counters plus the backlog, dead-letter, and lag gauges. Without one
// the outbox records nothing, so a stalled outbox is indistinguishable from an
// idle one. See [outbox.WithCollector] and docs/metrics.md.
func WithCollector(c metrics.Collector) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithCollector(c))
	}
}

// WithPublishedEventsLifetime sets how long successfully published messages are retained
// in the store before the cleanup cycle deletes them.
func WithPublishedEventsLifetime(t time.Duration) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithPublishedEventsLifetime(t))
	}
}

// WithTopicCompaction enables topic compaction for all topics.
// When enabled, only the latest message per topic is kept in a dispatch batch,
// reducing duplicate processing when multiple updates to the same entity are pending.
func WithTopicCompaction() Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithCompaction())
	}
}

// WithTopicCompactionFilter enables topic compaction only for topics where fn returns true.
// See [WithTopicCompaction] for a description of the compaction behavior.
func WithTopicCompactionFilter(fn func(string) bool) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithCompactionFilter(fn))
	}
}

// WithScheduler sets the scheduler for registering background tasks
// (dispatch, unlock, and cleanup cycles).
func WithScheduler(v corescheduler.TaskRegistrar) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithScheduler(v))
	}
}

// WithDispatchSchedule sets the cron schedule for the dispatch cycle that fetches
// pending messages and publishes them to the broker.
func WithDispatchSchedule[T interface{ string | *string }](v T) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithDispatchSchedule(v))
	}
}

// WithUnlockSchedule sets the cron schedule for the unlock cycle that releases
// messages stuck in [outbox.StatusInProgress] (e.g., after a crash during dispatch).
func WithUnlockSchedule[T interface{ string | *string }](v T) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithUnlockSchedule(v))
	}
}

// WithCleanupSchedule sets the cron schedule for the cleanup cycle that deletes
// successfully published messages older than the configured lifetime.
func WithCleanupSchedule[T interface{ string | *string }](v T) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithCleanupSchedule(v))
	}
}

// WithDefaultEventTTL sets the default time-to-live for messages without an explicit ExpiresAt.
// A value of 0 (default) means messages never expire based on time.
func WithDefaultEventTTL(d time.Duration) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithDefaultEventTTL(d))
	}
}

// WithExpireSchedule sets the cron schedule for the expire cycle that marks
// pending/failed messages past their ExpiresAt as expired.
func WithExpireSchedule[T interface{ string | *string }](v T) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithExpireSchedule(v))
	}
}

// WithStatsSchedule sets the cron schedule for the stats cycle that refreshes
// the backlog, dead-letter, and lag gauges. Leaving it unset leaves those gauges
// at zero, which reads exactly like a healthy idle outbox.
func WithStatsSchedule[T interface{ string | *string }](v T) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithStatsSchedule(v))
	}
}

// Scheduler task IDs. Override them when more than one outbox shares a
// scheduler: the registrar upserts by ID, so two instances on the defaults
// would silently overwrite each other's task functions. Collisions across the
// five IDs are rejected at startup with [outbox.ErrTaskIDCollision].

// WithDispatchTaskID overrides the scheduler task ID of the dispatch cycle.
func WithDispatchTaskID[T interface{ string | *string }](v T) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithDispatchTaskID(v))
	}
}

// WithUnlockTaskID overrides the scheduler task ID of the unlock cycle.
func WithUnlockTaskID[T interface{ string | *string }](v T) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithUnlockTaskID(v))
	}
}

// WithExpireTaskID overrides the scheduler task ID of the expire cycle.
func WithExpireTaskID[T interface{ string | *string }](v T) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithExpireTaskID(v))
	}
}

// WithCleanupTaskID overrides the scheduler task ID of the cleanup cycle.
func WithCleanupTaskID[T interface{ string | *string }](v T) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithCleanupTaskID(v))
	}
}

// WithStatsTaskID overrides the scheduler task ID of the stats cycle.
func WithStatsTaskID[T interface{ string | *string }](v T) Option {
	return func(o *adapterOptions) {
		o.genericOpts = append(o.genericOpts, outbox.WithStatsTaskID(v))
	}
}
