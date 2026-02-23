// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import (
	"context"
	"log/slog"
	"time"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// Default values for Outbox configuration.
const (
	// DefaultPublishedEventsLifetime is the default retention duration (-1 = indefinite).
	DefaultPublishedEventsLifetime = -1

	// MinPublishedEventsLifetime is the minimum allowed retention duration.
	// Values below this threshold are ignored to prevent accidental data loss.
	MinPublishedEventsLifetime = time.Minute

	// DefaultLockInterval is the maximum lock time before an event is considered stuck (10 seconds).
	DefaultLockInterval = 10 * time.Second

	// DefaultEventsBatchSize is the default events fetched per batch (200).
	DefaultEventsBatchSize = 200

	// DefaultRetryMaxAttempts is the default maximum dispatch attempts (10).
	DefaultRetryMaxAttempts = 10

	// DefaultRetryInterval is the internal retry interval (5 seconds).
	DefaultRetryInterval = 5 * time.Second

	// DefaultFetchTimeout is the default Store.FetchUnprocessedEvents timeout (5 seconds).
	DefaultFetchTimeout = 5 * time.Second

	// DefaultHandleTimeout is the default per-cycle handleEvents timeout (20 seconds).
	DefaultHandleTimeout = 20 * time.Second

	// DefaultUpdateTimeout is the default Store.UpdateEvents timeout (5 seconds).
	DefaultUpdateTimeout = 5 * time.Second
)

// options contains configuration fields for Outbox that can be set via Option functions.
type options struct {
	// Timeouts
	fetchTimeout  time.Duration `optval:"positive" optgen:"default=DefaultFetchTimeout"`
	handleTimeout time.Duration `optval:"positive" optgen:"default=DefaultHandleTimeout"`
	updateTimeout time.Duration `optval:"positive" optgen:"default=DefaultUpdateTimeout"`

	// Batch and retry settings
	eventsBatchSize  uint32 `optval:"positive" optgen:"default=DefaultEventsBatchSize"`
	retryMaxAttempts uint32 `optval:"positive" optgen:"default=DefaultRetryMaxAttempts"`

	// Logger
	logger *slog.Logger

	// Context
	baseCtx context.Context `opt:"Context" optval:"notnil"`

	// Published events lifetime - handled manually due to custom validation
	publishedEventsLifetime time.Duration `opt:"-" optgen:"default=DefaultPublishedEventsLifetime"`

	// Key compaction - handled manually
	compaction       bool              `opt:"-"`
	compactionFilter func(string) bool `opt:"-"`

	// Scheduler configuration
	scheduler        corescheduler.TaskRegistrar `optgen:"notnil"`
	dispatchSchedule string
	unlockSchedule   string
	cleanupSchedule  string

	// ShouldRetry determines whether a failed dispatch should be retried.
	// Return true to retry, false to stop retrying. If nil, all errors
	// except context.Canceled are retried.
	shouldRetry func(error) bool `opt:"-"`
}

// WithPublishedEventsLifetime sets how long published events are retained before cleanup.
// Values less than MinPublishedEventsLifetime are ignored to prevent accidental data loss.
// Use DefaultPublishedEventsLifetime (-1) to disable cleanup entirely.
func WithPublishedEventsLifetime(t time.Duration) Option {
	return func(o *options) {
		if t < MinPublishedEventsLifetime {
			return
		}
		o.publishedEventsLifetime = t
	}
}

// WithCompaction enables log compaction behavior for ALL keys.
// When enabled, if multiple events with the same Key are fetched in a single batch,
// only the event with the latest CreatedAt timestamp will be dispatched.
// Other events with the same Key will be marked as skipped without actual dispatching.
//
// Useful for scenarios where rapid state changes (e.g., pending -> processing -> completed)
// result in multiple events for the same entity, but only the final state matters.
//
// If keys contain a unique entity identifier (e.g., "orders.123", "users.abc-def"),
// compaction works per-entity - only the latest event for each entity is dispatched.
// If keys are generic without unique identifiers (e.g., "orders.created"),
// all events with that key in the batch will be grouped together,
// and only one event (the latest) will be dispatched.
func WithCompaction() Option {
	return func(o *options) {
		o.compaction = true
		o.compactionFilter = nil // nil means all keys
	}
}

// WithCompactionFilter enables selective log compaction based on a filter function.
// Only keys that pass the filter function will be compacted.
// This allows fine-grained control over which keys should be deduplicated.
//
// The filter function receives the key string and returns true if the key
// should be subject to compaction, false otherwise.
//
// Common patterns:
//   - Exact match: func(k string) bool { return k == "orders.123" }
//   - Prefix match: func(k string) bool { return strings.HasPrefix(k, "orders.") }
//   - Whitelist: func(k string) bool { return slices.Any(compactKeys, func(ck string) bool { return ck == k }) }
//   - Regex: func(k string) bool { return regexp.MustCompile(`^orders\.\d+$`).MatchString(k) }
func WithCompactionFilter(fn func(string) bool) Option {
	return func(o *options) {
		o.compaction = true
		o.compactionFilter = fn
	}
}

// WithShouldRetry sets a function that determines whether a failed dispatch should be retried.
// Return true to retry, false to stop retrying immediately.
//
// This is useful for transport-specific non-retryable errors. For example, a NATS adapter
// might use this to stop retrying on nats.ErrConnectionClosed.
//
// If not set, all errors except context.Canceled are retried.
func WithShouldRetry(fn func(error) bool) Option {
	return func(o *options) {
		o.shouldRetry = fn
	}
}
