// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"cmp"
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreretry "github.com/altessa-s/go-atlas/core/retry"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// Outbox implements the transactional outbox pattern for reliable event delivery.
// It persists events to a Store before dispatching via a Handler.
// Background goroutines handle dispatching, retries, and maintenance.
type Outbox struct {
	store   Store   // The underlying storage system for events.
	handler Handler // The function responsible for dispatching events.

	// Configuration fields set via Options:
	publishedEventsLifetime time.Duration
	maxLockTime             time.Duration
	fetchTimeout            time.Duration
	handleTimeout           time.Duration
	updateTimeout           time.Duration
	compactionFilter        func(string) bool // Optional filter for selective compaction (nil = all keys).
	baseCtx                 context.Context
	shouldRetry             func(error) bool        // Optional caller-provided retry predicate.
	retryBackoff            coreretry.NextDelayFunc // Exponential backoff with jitter between attempts.

	// Metrics
	metrics *outboxMetrics

	// Internal state:
	logger    *slog.Logger // Internal logger.
	scheduler corescheduler.TaskRegistrar

	dispatchTask corescheduler.ManagedTask // Guards RunDispatchCycle and marks scheduler management.
	unlockTask   corescheduler.ManagedTask // Guards RunUnlockCycle and marks scheduler management.
	cleanupTask  corescheduler.ManagedTask // Guards RunCleanupCycle and marks scheduler management.
	expireTask   corescheduler.ManagedTask // Guards RunExpireCycle and marks scheduler management.
	statsTask    corescheduler.ManagedTask // Guards RunStatsCycle and marks scheduler management.

	retryMaxAttempts uint32
	eventsBatchSize  uint32
	maxPayloadBytes  int
	compaction       bool          // When true, key compaction is enabled.
	defaultEventTTL  time.Duration // Default TTL for events without explicit ExpiresAt.
}

// New creates a new Outbox with the given Store and Handler.
// Default settings are applied and can be overridden via Option functions.
func New(store Store, handler Handler, opts ...Option) *Outbox {
	cfg := newOptions(opts...)

	// Apply nil defaults for fields that optgen doesn't handle
	cfg.baseCtx = cmp.Or(cfg.baseCtx, context.Background())
	cfg.logger = cmp.Or(cfg.logger, slog.New(slog.DiscardHandler))

	// A lock that expires while the cycle holding it is still publishing lets
	// the unlock sweeper hand the same event to a second worker, so duplicate
	// delivery stops being an edge case and becomes the steady state. Clamp
	// rather than reject: a running service with a mildly wrong knob should
	// keep working, loudly.
	maxLockTime := cfg.maxLockTime
	if maxLockTime <= cfg.handleTimeout {
		maxLockTime = lockTimeHandleTimeoutRatio * cfg.handleTimeout
		cfg.logger.Warn("outbox lock time must exceed the handle timeout; raising it to avoid duplicate dispatch",
			slog.Duration("configured_max_lock_time", cfg.maxLockTime),
			slog.Duration("handle_timeout", cfg.handleTimeout),
			slog.Duration("effective_max_lock_time", maxLockTime),
		)
	}

	o := &Outbox{
		metrics:                 newOutboxMetrics(cfg.collector),
		eventsBatchSize:         cfg.eventsBatchSize,
		publishedEventsLifetime: cfg.publishedEventsLifetime,
		retryMaxAttempts:        cfg.retryMaxAttempts,
		maxLockTime:             maxLockTime,
		fetchTimeout:            cfg.fetchTimeout,
		handleTimeout:           cfg.handleTimeout,
		updateTimeout:           cfg.updateTimeout,
		store:                   store,
		handler:                 handler,
		logger:                  cfg.logger,
		baseCtx:                 cfg.baseCtx,
		compaction:              cfg.compaction,
		compactionFilter:        cfg.compactionFilter,
		scheduler:               cfg.scheduler,
		shouldRetry:             cfg.shouldRetry,
		defaultEventTTL:         cfg.defaultEventTTL,
		maxPayloadBytes:         cfg.maxPayloadBytes,
		retryBackoff: coreretry.Exponential(coreretry.ExponentialConfig{
			BaseDelay: cfg.retryBaseDelay,
			MaxDelay:  cfg.retryMaxDelay,
			Factor:    DefaultRetryFactor,
			Jitter:    DefaultRetryJitter,
		}),
	}

	// Register outbox tasks with scheduler if provided
	if err := o.registerTasks(cfg); err != nil {
		// Log error but don't fail creation - task registration is optional
		cfg.logger.Warn("failed to register outbox tasks", slog.Any("error", err))
	}

	return o
}

// Save persists events to the outbox store for later dispatching.
// Events without an Id will be assigned a UUID automatically.
//
// Save MUST run inside the same store transaction as the business data it
// describes — that atomicity is the entire point of the pattern. With the
// MongoDB store that means passing the session context:
//
//	_, err := sess.WithTransaction(ctx, func(sessCtx context.Context) (any, error) {
//	    if err := repo.CreateOrder(sessCtx, order); err != nil {
//	        return nil, err
//	    }
//	    return nil, ob.Save(sessCtx, outbox.Event{Key: "billing.order.created", Payload: payload})
//	})
//
// Passing a plain context instead reintroduces the dual-write gap the outbox
// exists to close: the order can commit while the event is lost, or the
// reverse.
//
// Events are validated before any store call, so a rejected batch leaves the
// caller's transaction intact and abortable.
func (o *Outbox) Save(ctx context.Context, events ...Event) error {
	if len(events) == 0 {
		return nil
	}

	for i := range events {
		if events[i].Key == "" {
			return coreerrs.Wrapf(ErrEmptyKey, "event at index %d", i)
		}
		if o.maxPayloadBytes > 0 && len(events[i].Payload) > o.maxPayloadBytes {
			return coreerrs.Wrapf(ErrPayloadTooLarge, "event %q: %d bytes exceeds the %d byte limit; use a claim-check",
				events[i].Key, len(events[i].Payload), o.maxPayloadBytes)
		}
	}

	now := time.Now().UTC()
	for i := range events {
		if events[i].Id == "" {
			events[i].Id = uuid.New().String()
		}
		if events[i].Status == "" {
			events[i].Status = StatusPending
		}
		if events[i].CreatedAt.IsZero() {
			events[i].CreatedAt = now
		}
		if events[i].ExpiresAt.IsZero() && o.defaultEventTTL > 0 {
			events[i].ExpiresAt = events[i].CreatedAt.Add(o.defaultEventTTL)
		}
		if !events[i].ExpiresAt.IsZero() && events[i].ExpiresAt.Before(now) {
			o.logger.WarnContext(ctx, "event saved with ExpiresAt in the past; it will never be dispatched",
				slog.String("event_id", events[i].Id),
				slog.Time("expires_at", events[i].ExpiresAt),
			)
		}
	}

	if err := o.store.SaveEvents(ctx, events...); err != nil {
		return err
	}
	o.metrics.eventsSaved.Add(float64(len(events)))
	return nil
}

// --- Internal Methods ---

// dispatchEvent makes exactly one delivery attempt for the event.
//
// Retrying is deliberately not done here. An in-process retry loop holds the
// event's store lock for the whole loop, so the lock expires mid-flight and the
// unlock sweeper hands the event to a second worker; it also multiplies the
// attempt budget, because the outer state machine counts one attempt per cycle
// while the inner loop burns several. Spacing between attempts is instead
// expressed as [Event.RetryAfter] and enforced by the store, which keeps the
// backoff durable across restarts and consistent across instances.
func (o *Outbox) dispatchEvent(ctx context.Context, event Event) error {
	o.metrics.eventsInFlight.Inc()
	defer o.metrics.eventsInFlight.Dec()

	if err := o.handler(ctx, event); err != nil {
		o.metrics.eventsDispatchFail.Inc()
		return err
	}

	o.metrics.eventsDispatched.Inc()
	return nil
}

// isRetryable classifies a dispatch failure as transient (retry later) or
// permanent (dead-letter now). A canceled or timed-out context means our own
// cycle was cut short — the broker never returned a verdict about the event —
// so it always counts as transient, whatever the caller's predicate says.
// Otherwise a broker outage would dead-letter perfectly good events.
func (o *Outbox) isRetryable(err error) bool {
	if coreerrs.IsContextCanceled(err) || coreerrs.IsContextDeadlineExceeded(err) {
		return true
	}
	if o.shouldRetry != nil {
		return o.shouldRetry(err)
	}
	return true
}

// compactEventsByKey applies log compaction to keep only the latest event per key.
// Returns two slices: events to dispatch (latest per key) and events to skip (older duplicates).
//
// The batch is sorted by CreatedAt ascending first. [Store] is required to
// return it that way already, but compaction is the one place where a violated
// ordering contract would be actively harmful rather than merely untidy — it
// would dispatch a stale event and discard the current one, silently — so the
// invariant is re-established here instead of assumed.
//
// If compactionFilter is set, only events matching the filter will be compacted.
// Events not matching the filter are always dispatched (bypass compaction).
//
// This function groups events by the exact Key string value.
// If keys contain a unique entity identifier (e.g., "orders.123", "users.abc-def"),
// compaction works per-entity - only the latest event for each entity is dispatched.
// If keys are generic without unique identifiers (e.g., "orders.created"),
// all events with that key in the batch will be grouped together,
// and only one event (the latest) will be dispatched.
func (o *Outbox) compactEventsByKey(events []Event) (toPublish, toSkip []Event) {
	if len(events) == 0 {
		return nil, nil
	}

	// Stable so events sharing a CreatedAt keep their fetch order, making the
	// choice of survivor deterministic rather than dependent on sort internals.
	slices.SortStableFunc(events, func(a, b Event) int { return a.CreatedAt.Compare(b.CreatedAt) })

	// If filter is set, separate events into compactable and non-compactable
	var compactableEvents []Event
	var nonCompactableEvents []Event

	if o.compactionFilter != nil {
		// Single partition pass instead of two separate Filter+Collect calls.
		compactableEvents = make([]Event, 0, len(events))
		nonCompactableEvents = make([]Event, 0, len(events)/2) //nolint:mnd // rough estimate
		for _, e := range events {
			if o.compactionFilter(e.Key) {
				compactableEvents = append(compactableEvents, e)
			} else {
				nonCompactableEvents = append(nonCompactableEvents, e)
			}
		}
	} else {
		// No filter = all events are compactable
		compactableEvents = events
	}

	// Group compactable events by key using core/collections/slices.GroupBy
	grouped := coreslices.GroupBy(compactableEvents, func(e Event) string { return e.Key })

	toPublish = make([]Event, 0, len(grouped)+len(nonCompactableEvents))
	toSkip = make([]Event, 0, max(len(compactableEvents)-len(grouped), 0))

	// Add non-compactable events (they always get dispatched)
	toPublish = append(toPublish, nonCompactableEvents...)

	// Process compactable events: keep only the latest per key, skip the rest.
	// Direct range avoids intermediate slices from Collect/Map/Filter.
	for _, group := range grouped {
		toPublish = append(toPublish, group[len(group)-1])
		toSkip = coreslices.AppendIf(toSkip, len(group) > 1, group[:len(group)-1]...)
	}

	return toPublish, toSkip
}

// handleEvents processes events concurrently and updates their status in the Store.
func (o *Outbox) handleEvents(ctx context.Context, events ...Event) {
	// Apply key compaction if enabled.
	// Events to skip are marked as skipped without dispatching.
	var skippedEvents []Event
	if o.compaction {
		var toPublish []Event
		toPublish, skippedEvents = o.compactEventsByKey(events)

		// Mark skipped events with appropriate status.
		for i := range skippedEvents {
			skippedEvents[i].setSkippedStatus()
		}

		o.metrics.eventsSkipped.Add(float64(len(skippedEvents)))

		o.logger.DebugContext(ctx, "key compaction applied",
			slog.Int("total", len(events)),
			slog.Int("to_dispatch", len(toPublish)),
			slog.Int("skipped", len(skippedEvents)),
		)

		events = toPublish
	}

	// The transform never returns an error, and that is load-bearing:
	// ProcessCollect drops the transformed value of any item whose function
	// failed. Signaling a failed dispatch through the error return therefore
	// discarded exactly the events whose new state mattered most — the failure
	// was never written back, so attempts never grew, the error was never
	// recorded, and no event could ever reach a terminal status. The outcome
	// travels on the event's own status instead; per-event errors are logged
	// where they happen.
	processedEvents, err := concurrency.ProcessCollect(ctx, events, func(ctx context.Context, event Event) (Event, error) {
		event.nextAttempt() // Increment attempts etc. on the copy

		logger := o.logger.With(
			slog.Group("event", "id", event.Id, "key", event.Key, "attempts", event.Attempts),
		)

		// Attempt to dispatch the event using the context.
		if err := o.dispatchEvent(ctx, event); err != nil {
			event.setErrorStatus(err)

			switch {
			case !o.isRetryable(err):
				// Repeating a permanent failure cannot fix it and would only
				// delay the operator seeing it, so dead-letter immediately
				// instead of spending the remaining attempt budget.
				event.setRejectedStatus()
				o.metrics.eventsRejected.Inc()
				logger.ErrorContext(ctx, "event rejected as permanently undeliverable", slog.Any("error", err))

			case !event.isReadyForRetry(o.retryMaxAttempts):
				event.setStatusMaxAttemptReached()
				o.metrics.maxRetriesExhausted.Inc()
				logger.ErrorContext(ctx, "event dead-lettered after exhausting the retry budget", slog.Any("error", err))

			default:
				// Exponential backoff with jitter, keyed off the attempt
				// count. It travels to the store as a duration so the
				// deadline is anchored to the backend clock.
				event.RetryAfter = o.retryBackoff(int(event.Attempts)-1, err)
				o.metrics.dispatchRetries.Inc()
				logger.WarnContext(ctx, "failed to dispatch event, scheduling retry",
					slog.Any("error", err),
					slog.Duration("next_try_in", event.RetryAfter),
				)
			}

			return event, nil
		}

		// Success
		event.setSentStatus()
		logger.DebugContext(ctx, "event has been dispatched")
		return event, nil
	})

	if err != nil {
		// Not a dispatch failure — the transform cannot fail. This is the
		// batch being cut short (a canceled cycle), so some events were never
		// attempted. They keep their lock and return to the queue when the
		// unlock sweeper reclaims them.
		o.logger.WarnContext(ctx, "batch processing stopped before every event was attempted", slog.Any("error", err))
	}

	// Combine processed and skipped events for store update.
	processedEvents = append(processedEvents, skippedEvents...)

	// Update the final statuses of all processed events in the store.
	if len(processedEvents) > 0 {
		updateCtx, cancelUpdate := context.WithTimeout(context.WithoutCancel(ctx), o.updateTimeout)
		defer cancelUpdate()

		if updateErr := o.store.UpdateEvents(updateCtx, processedEvents...); updateErr != nil {
			o.logger.ErrorContext(ctx, "failed to update processed event statuses in store", slog.Any("error", updateErr))
		}
	} else {
		o.logger.DebugContext(ctx, "handleEvents finished, no events were collected for update") // Should not happen if events were passed in
	}
}
