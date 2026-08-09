// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"context"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// Default values for Outbox configuration.
const (
	// DefaultPublishedEventsLifetime is the default retention duration (-1 = indefinite).
	DefaultPublishedEventsLifetime = -1

	// MinPublishedEventsLifetime is the minimum allowed retention duration.
	// Values below this threshold are ignored to prevent accidental data loss.
	MinPublishedEventsLifetime = time.Minute

	// DefaultLockInterval is the maximum lock time before an event is considered
	// stuck. It MUST exceed DefaultHandleTimeout: a lock that expires while the
	// dispatch cycle that holds it is still publishing lets the unlock sweeper
	// hand the event to a second worker, turning duplicate delivery from a
	// failure mode into the steady state. New enforces the same relation for
	// caller-supplied values.
	DefaultLockInterval = lockTimeHandleTimeoutRatio * DefaultHandleTimeout

	// lockTimeHandleTimeoutRatio is the headroom the lock keeps over the
	// dispatch cycle it protects: enough that a cycle running to its full
	// timeout still finishes inside its lease, without pinning an event for
	// so long that a genuinely dead worker stalls it.
	lockTimeHandleTimeoutRatio = 2

	// DefaultEventsBatchSize is the default events fetched per batch (200).
	DefaultEventsBatchSize = 200

	// DefaultRetryMaxAttempts is the default maximum dispatch attempts (10).
	DefaultRetryMaxAttempts = 10

	// DefaultFetchTimeout is the default Store.FetchUnprocessedEvents timeout (5 seconds).
	DefaultFetchTimeout = 5 * time.Second

	// DefaultHandleTimeout is the default per-cycle handleEvents timeout (20 seconds).
	DefaultHandleTimeout = 20 * time.Second

	// DefaultUpdateTimeout is the default Store.UpdateEvents timeout (5 seconds).
	DefaultUpdateTimeout = 5 * time.Second

	// DefaultMaxPayloadBytes caps Event.Payload at 1 MiB. Larger messages belong
	// behind a claim-check — an object-store reference in the payload — rather
	// than in the outbox and the broker, both of which have hard document and
	// message size limits. Pass 0 to WithMaxPayloadBytes to disable the check.
	DefaultMaxPayloadBytes = 1 << 20

	// DefaultDispatchTaskID is the scheduler task ID for the dispatch cycle.
	DefaultDispatchTaskID = "outbox-dispatch"

	// DefaultUnlockTaskID is the scheduler task ID for the unlock cycle.
	DefaultUnlockTaskID = "outbox-unlock"

	// DefaultExpireTaskID is the scheduler task ID for the expire cycle.
	DefaultExpireTaskID = "outbox-expire"

	// DefaultCleanupTaskID is the scheduler task ID for the cleanup cycle.
	DefaultCleanupTaskID = "outbox-cleanup"

	// DefaultStatsTaskID is the scheduler task ID for the stats cycle.
	DefaultStatsTaskID = "outbox-stats"
)

// Retry backoff defaults for events that failed to dispatch.
//
// The delay is computed per event from its attempt count and handed to the
// Store as a duration, which anchors it to the backend clock. A flat retry
// interval would keep hammering an already failing dependency at a constant
// rate; without jitter, every event that failed in the same cycle would come
// back due at the same instant and arrive as a wave.
const (
	// DefaultRetryBaseDelay is the backoff before the second attempt.
	DefaultRetryBaseDelay = time.Second

	// DefaultRetryMaxDelay caps the backoff between attempts.
	DefaultRetryMaxDelay = 5 * time.Minute

	// DefaultRetryFactor is the exponential multiplier applied per attempt.
	DefaultRetryFactor = 1.5

	// DefaultRetryJitter randomizes each backoff by up to this fraction of itself.
	DefaultRetryJitter = 0.2
)

// options contains configuration fields for Outbox that can be set via Option functions.
type options struct {
	// Timeouts
	fetchTimeout  time.Duration `optval:"positive" optgen:"default=DefaultFetchTimeout"`
	handleTimeout time.Duration `optval:"positive" optgen:"default=DefaultHandleTimeout"`
	updateTimeout time.Duration `optval:"positive" optgen:"default=DefaultUpdateTimeout"`

	// Batch and retry settings
	eventsBatchSize  uint32        `optval:"positive" optgen:"default=DefaultEventsBatchSize"`
	retryMaxAttempts uint32        `optval:"positive" optgen:"default=DefaultRetryMaxAttempts"`
	retryBaseDelay   time.Duration `optval:"positive" optgen:"default=DefaultRetryBaseDelay"`
	retryMaxDelay    time.Duration `optval:"positive" optgen:"default=DefaultRetryMaxDelay"`

	// maxLockTime bounds how long an event may stay locked before the unlock
	// sweeper reclaims it. Must exceed handleTimeout — see DefaultLockInterval.
	maxLockTime time.Duration `optval:"positive" optgen:"default=DefaultLockInterval"`

	// Logger
	logger *slog.Logger

	// Context
	baseCtx context.Context `opt:"Context" optgen:"notnil"`

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
	expireSchedule   string
	statsSchedule    string

	// Scheduler task IDs — overridable so multiple Outbox instances can coexist
	// in a single scheduler without ID collisions. The generated WithXxx
	// setters TrimSpace the input and treat an empty / whitespace-only string
	// as a no-op (the default is kept) — this matches the repo-wide optgen
	// string-setter convention. Pass distinct non-empty values when running
	// more than one Outbox against the same scheduler; collisions are
	// rejected at registerTasks time via [ErrTaskIDCollision].
	dispatchTaskID string `optgen:"default=DefaultDispatchTaskID"`
	unlockTaskID   string `optgen:"default=DefaultUnlockTaskID"`
	expireTaskID   string `optgen:"default=DefaultExpireTaskID"`
	cleanupTaskID  string `optgen:"default=DefaultCleanupTaskID"`
	statsTaskID    string `optgen:"default=DefaultStatsTaskID"`

	// ShouldRetry determines whether a failed dispatch should be retried.
	// Return true to retry, false to stop retrying. If nil, all errors
	// except context.Canceled are retried.
	shouldRetry func(error) bool `opt:"-"`

	// Default TTL applied to events at save time when ExpiresAt is not set.
	// 0 means disabled (no expiration). Minimum 1s.
	defaultEventTTL time.Duration `opt:"-"`

	// Maximum accepted Event.Payload size in bytes; 0 disables the check.
	// Handled manually because 0 is a meaningful value, not a "keep default".
	maxPayloadBytes int `opt:"-" optgen:"default=DefaultMaxPayloadBytes"`

	// Metrics collector for outbox instrumentation.
	collector metrics.Collector `optgen:"notnil"`
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

// WithShouldRetry sets the predicate that separates transient failures from
// permanent ones. Return true to retry, false to dead-letter the event
// immediately with [StatusRejected].
//
// Use it for errors that another attempt cannot fix — a payload the transport
// refuses to encode, an unknown subject, a 4xx from the destination. A timeout
// or a dropped connection is transient and should return true.
//
// If not set, every error is treated as transient and retried until the
// attempt budget set by [WithRetryMaxAttempts] runs out.
func WithShouldRetry(fn func(error) bool) Option {
	return func(o *options) {
		o.shouldRetry = fn
	}
}

// WithMaxPayloadBytes caps the accepted size of [Event.Payload]. Save rejects
// larger events with [ErrPayloadTooLarge] instead of letting them fail deep in
// the store driver — inside the caller's business transaction, where the error
// is far more expensive. Pass 0 to disable the check.
//
// Defaults to [DefaultMaxPayloadBytes]. Payloads approaching the limit should
// use a claim-check: put the blob in object storage and publish a reference.
func WithMaxPayloadBytes(n int) Option {
	return func(o *options) {
		if n < 0 {
			return
		}
		o.maxPayloadBytes = n
	}
}

// WithDefaultEventTTL sets the default time-to-live applied to events at save time
// when their ExpiresAt is not explicitly set. Values below 1s are ignored (disabled).
// A value of 0 (default) means events never expire based on time.
func WithDefaultEventTTL(d time.Duration) Option {
	return func(o *options) {
		if d < time.Second {
			return
		}
		o.defaultEventTTL = d
	}
}
