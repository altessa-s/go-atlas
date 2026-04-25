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
	e.PublishedAt = time.Now().UTC()
}

// setStatusMaxAttemptReached sets status to StatusMaxAttemptReached.
func (e *Event) setStatusMaxAttemptReached() {
	e.Status = StatusMaxAttemptReached
}

// setSkippedStatus updates the event's state when it is intentionally not dispatched.
// It sets the Status to StatusSkipped, clears any LastError, and records PublishedAt
// to enable cleanup of processed events.
func (e *Event) setSkippedStatus() {
	e.Status = StatusSkipped
	e.LastError = nil
	e.PublishedAt = time.Now().UTC()
}

// setExpiredStatus marks the event as expired, clears LastError,
// and records PublishedAt for cleanup eligibility.
func (e *Event) setExpiredStatus() {
	e.Status = StatusExpired
	e.LastError = nil
	e.PublishedAt = time.Now().UTC()
}

// isReadyForRetry returns true if attempts are below maxAttempts.
func (e *Event) isReadyForRetry(maxAttempts uint32) bool {
	return e.Attempts < maxAttempts
}
