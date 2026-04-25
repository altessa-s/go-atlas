// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"cmp"
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreretry "github.com/altessa-s/go-atlas/core/retry"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// Default retry configuration for event dispatching.
const (
	// DefaultDispatchRetryBaseDelay is the initial delay between retry attempts.
	// Uses exponential backoff starting from this base delay.
	DefaultDispatchRetryBaseDelay = 500 * time.Millisecond

	// DefaultDispatchRetryMaxDelay is the maximum delay between retry attempts.
	// Exponential backoff will not exceed this value.
	DefaultDispatchRetryMaxDelay = 3 * time.Second
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
	retryInterval           time.Duration
	fetchTimeout            time.Duration
	handleTimeout           time.Duration
	updateTimeout           time.Duration
	compactionFilter        func(string) bool // Optional filter for selective compaction (nil = all keys).
	baseCtx                 context.Context
	shouldRetry             func(error) bool // Optional caller-provided retry predicate.

	// Metrics
	metrics *outboxMetrics

	// Internal state:
	eventsInFlight  atomic.Int64 // Counts events currently being processed by handleEvents.
	logger          *slog.Logger // Internal logger.
	scheduler       corescheduler.TaskRegistrar
	dispatchRunning atomic.Bool // Guards against concurrent RunDispatchCycle calls.
	unlockRunning   atomic.Bool // Guards against concurrent RunUnlockCycle calls.
	cleanupRunning  atomic.Bool // Guards against concurrent RunCleanupCycle calls.

	schedulerDispatchRegistered atomic.Bool // Marks if RunDispatchCycle is managed by scheduler.
	schedulerUnlockRegistered   atomic.Bool // Marks if RunUnlockCycle is managed by scheduler.
	schedulerCleanupRegistered  atomic.Bool // Marks if RunCleanupCycle is managed by scheduler.
	schedulerExpireRegistered   atomic.Bool // Marks if RunExpireCycle is managed by scheduler.
	expireRunning               atomic.Bool // Guards against concurrent RunExpireCycle calls.

	retryMaxAttempts uint32
	eventsBatchSize  uint32
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

	o := &Outbox{
		metrics:                 newOutboxMetrics(cfg.collector),
		eventsBatchSize:         cfg.eventsBatchSize,
		publishedEventsLifetime: cfg.publishedEventsLifetime,
		retryInterval:           DefaultRetryInterval,
		retryMaxAttempts:        cfg.retryMaxAttempts,
		maxLockTime:             DefaultLockInterval,
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
	}

	// Register outbox tasks with scheduler if provided
	if err := o.registerTasks(cfg); err != nil {
		// Log error but don't fail creation - task registration is optional
		cfg.logger.Warn("failed to register outbox tasks", slog.Any("error", err))
	}

	return o
}

// Save persists events to the outbox store for later dispatching.
// Should be called within the same transaction as the business logic for atomicity.
// Events without an Id will be assigned a UUID automatically.
//
// Example:
//
//	err := ob.Save(ctx, outbox.Event{Key: "events", Payload: payload})
func (o *Outbox) Save(ctx context.Context, events ...Event) error {
	if len(events) == 0 {
		return nil
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

// StopInternalProcesses is a no-op as all background tasks are managed externally.
func (o *Outbox) StopInternalProcesses() {}

// --- Internal Methods ---

// dispatchEvent attempts to dispatch a single event with exponential backoff retry.
// Decrements eventsInFlight counter upon completion.
func (o *Outbox) dispatchEvent(ctx context.Context, event Event) error {
	// Decrement the in-flight counter when this function exits.
	defer o.eventsInFlight.Add(-1)

	o.metrics.eventsInFlight.Inc()
	defer o.metrics.eventsInFlight.Dec()

	err := coreretry.Do(ctx, func(ctx context.Context) error {
		return o.handler(ctx, event)
	},
		coreretry.WithMaxAttempts(-1), // retry until success or ctx cancellation
		coreretry.WithShouldRetry(func(err error) bool {
			if coreerrs.IsContextCanceled(err) {
				return false
			}
			if o.shouldRetry != nil {
				return o.shouldRetry(err)
			}
			return true
		}),
		coreretry.WithNextDelay(coreretry.Exponential(coreretry.ExponentialConfig{
			BaseDelay: DefaultDispatchRetryBaseDelay,
			MaxDelay:  DefaultDispatchRetryMaxDelay,
		})),
		coreretry.WithOnRetry(func(_ int, err error, nextDelay time.Duration) {
			o.metrics.dispatchRetries.Inc()
			o.logger.WarnContext(ctx, "failed to dispatch event, retrying...",
				slog.Any("error", err),
				slog.String("event_id", event.Id),
				slog.Duration("next_try_in", nextDelay),
			)
		}),
	)
	if err == nil {
		o.metrics.eventsDispatched.Inc()
		return nil
	}

	o.metrics.eventsDispatchFail.Inc()
	if coreerrs.IsContextCanceled(err) {
		return coreerrs.Wrap(err, "context canceled before next retry")
	}
	return err
}

// compactEventsByKey applies log compaction to keep only the latest event per key.
// Events are assumed to be sorted by CreatedAt ascending (oldest first).
// Returns two slices: events to dispatch (latest per key) and events to skip (older duplicates).
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
		if len(group) > 1 {
			toSkip = append(toSkip, group[:len(group)-1]...)
		}
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

	// Increment in-flight counter for the whole batch.
	// Defer decrementing until all processing (including store update) is done.
	o.eventsInFlight.Add(int64(len(events)))

	processedEvents, err := concurrency.ProcessCollect(ctx, events, func(ctx context.Context, event Event) (Event, error) {
		event.nextAttempt() // Increment attempts etc. on the copy

		logger := o.logger.With(
			slog.Group("event", "id", event.Id, "key", event.Key, "attempts", event.Attempts),
		)

		// Attempt to dispatch the event using the context.
		if err := o.dispatchEvent(ctx, event); err != nil {
			event.setErrorStatus(err)
			logger.ErrorContext(ctx, "failed to dispatch event", slog.Any("error", err))
			if !event.isReadyForRetry(o.retryMaxAttempts) {
				event.setStatusMaxAttemptReached()
				o.metrics.maxRetriesExhausted.Inc()
			}
			return event, err
		}

		// Success
		event.setSentStatus()
		logger.DebugContext(ctx, "event has been dispatched")
		return event, nil
	})

	if err != nil {
		// Log the overall batch error if needed, although individual errors are logged above.
		o.logger.WarnContext(ctx, "batch processing completed with some errors", slog.Any("error", err))
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
