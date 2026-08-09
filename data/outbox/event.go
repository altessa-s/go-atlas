// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"time"

	"github.com/altessa-s/go-atlas/core/types/ptr"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Status represents the state of an event in the Outbox processing lifecycle.
type Status string

// Possible statuses for an event in the outbox.
const (
	// StatusInProgress indicates the event is currently being processed.
	StatusInProgress Status = "in-progress"
	// StatusPending indicates the event is awaiting processing.
	StatusPending Status = "pending"
	// StatusSent indicates the event was successfully dispatched.
	StatusSent Status = "sent"
	// StatusFailed indicates a dispatch attempt failed; may be retried.
	StatusFailed Status = "failed"
	// StatusMaxAttemptReached indicates maximum retry attempts exceeded.
	StatusMaxAttemptReached Status = "max-attempt-reached"
	// StatusRejected indicates the dispatch failed with an error that the
	// configured retry predicate classified as permanent (a malformed payload,
	// an unknown subject). Such an event is never retried: repeating it cannot
	// fix it, so it is dead-lettered immediately instead of burning the whole
	// attempt budget first.
	StatusRejected Status = "rejected"
	// StatusSkipped indicates that the event was intentionally not dispatched
	// because it was superseded by a newer event or excluded by processing rules.
	StatusSkipped Status = "skipped"
	// StatusExpired indicates the event exceeded its ExpiresAt deadline
	// before being successfully dispatched.
	StatusExpired Status = "expired"
)

// Event represents an event stored in the outbox with its delivery tracking state.
type Event struct {
	Id            string    // Unique identifier (UUID).
	Key           string    // Generic grouping/routing key (e.g., topic, entity ID).
	Payload       []byte    // Event payload (caller-serialized).
	Status        Status    // Current processing status.
	CreatedAt     time.Time // When the event was created.
	PublishedAt   time.Time // When successfully dispatched; zero if not yet.
	ExpiresAt     time.Time // Deadline after which the event should no longer be dispatched; zero means no expiration.
	LastError     *string   // Last error message; nil if successful.
	Attempts      uint32    // Number of dispatch attempts made.
	LastAttemptOn time.Time // Timestamp of most recent attempt.
	LockedOn      time.Time // When locked for processing; zero if unlocked.

	// RetryAfter is the backoff delay the [Outbox] computed for the next
	// attempt of a failed event. It is a duration, never an absolute
	// instant, so the [Store] can anchor it to its own backend clock and
	// stay clock-skew safe. Zero means "eligible immediately".
	RetryAfter time.Duration

	// LockToken is the fencing token the [Store] assigned when it locked the
	// event for this dispatch cycle. [Store.UpdateEvents] must apply a write
	// only while the stored token still matches, so a worker that lost its
	// lock to the unlock sweeper cannot overwrite the state of the worker
	// that picked the event up afterwards.
	LockToken string
}

// nextAttempt increments attempts counter and updates LastAttemptOn timestamp.
func (e *Event) nextAttempt() {
	e.Attempts++
	e.LastAttemptOn = time.Now().UTC()
	e.LockedOn = time.Time{} // Clear lock before new attempt
}

// setErrorStatus sets StatusFailed and records the error (unless context.Canceled).
func (e *Event) setErrorStatus(err error) {
	e.Status = StatusFailed
	// Avoid overwriting a more specific previous error with a generic "context canceled".
	if err != nil && !coreerrs.IsContextCanceled(err) {
		e.LastError = ptr.Wrap(err.Error())
	}
}

// setSentStatus sets StatusSent, clears LastError, and records PublishedAt.
func (e *Event) setSentStatus() {
	e.Status = StatusSent
	e.LastError = nil
	e.RetryAfter = 0
	e.PublishedAt = time.Now().UTC()
}

// setStatusMaxAttemptReached sets status to StatusMaxAttemptReached and clears
// the pending backoff — a dead-lettered event is not scheduled for another try.
func (e *Event) setStatusMaxAttemptReached() {
	e.Status = StatusMaxAttemptReached
	e.RetryAfter = 0
}

// setRejectedStatus marks the event as permanently undeliverable. Like
// [Event.setStatusMaxAttemptReached] it is terminal, so no backoff is
// scheduled. The failure itself is recorded by [Event.setErrorStatus], which
// every failure path runs first.
func (e *Event) setRejectedStatus() {
	e.Status = StatusRejected
	e.RetryAfter = 0
}

// setSkippedStatus updates the event's state when it is intentionally not dispatched.
// It sets the Status to StatusSkipped, clears any LastError, and records PublishedAt
// to enable cleanup of processed events.
func (e *Event) setSkippedStatus() {
	e.Status = StatusSkipped
	e.LastError = nil
	e.RetryAfter = 0
	e.PublishedAt = time.Now().UTC()
}

// isReadyForRetry returns true if attempts are below maxAttempts.
func (e *Event) isReadyForRetry(maxAttempts uint32) bool {
	return e.Attempts < maxAttempts
}
