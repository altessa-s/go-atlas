// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"time"
)

const (
	// DefaultWatchBufferSize is the default buffer size for watch event channels.
	DefaultWatchBufferSize = 10
)

// BufferOverflowPolicy defines how to handle events when the buffer is full.
type BufferOverflowPolicy int

const (
	// OverflowPolicyDropNewest drops the newest event when the buffer is full (default).
	// This is non-blocking and ensures the event producer is not delayed.
	OverflowPolicyDropNewest BufferOverflowPolicy = iota

	// OverflowPolicyDropOldest drops the oldest event to make room for new ones.
	// This ensures the most recent events are always delivered.
	OverflowPolicyDropOldest

	// OverflowPolicyBlock blocks until space is available or context is canceled.
	// Use with caution as this can slow down the event producer.
	OverflowPolicyBlock
)

// String returns a string representation of the overflow policy.
func (p BufferOverflowPolicy) String() string {
	switch p {
	case OverflowPolicyDropNewest:
		return "drop_newest"
	case OverflowPolicyDropOldest:
		return "drop_oldest"
	case OverflowPolicyBlock:
		return "block"
	default:
		return "unknown"
	}
}

// EventType represents the type of policy change event.
type EventType int

const (
	// EventTypePolicyUpdated indicates that policies were successfully updated.
	EventTypePolicyUpdated EventType = iota

	// EventTypePolicyError indicates an error occurred during policy update.
	EventTypePolicyError
)

// String returns a string representation of the event type.
func (e EventType) String() string {
	switch e {
	case EventTypePolicyUpdated:
		return "policy_updated"
	case EventTypePolicyError:
		return "policy_error"
	default:
		return "unknown"
	}
}

// PolicyEvent represents an event related to policy updates.
// Events are emitted when policies are reloaded or when errors occur.
type PolicyEvent struct {
	// Type indicates what kind of event occurred.
	Type EventType

	// Source is the name of the PolicySource that generated this event.
	Source string

	// Revision is the new policy revision (for update events).
	Revision string

	// PreviousRevision is the previous policy revision (for update events).
	PreviousRevision string

	// OccurredAt is when the event occurred.
	OccurredAt time.Time

	// Error contains details if this is an error event.
	Error error
}

// WatchOptions configures the behavior of policy watch operations.
type WatchOptions struct {
	// BufferSize is the size of the event channel buffer.
	// Defaults to 10 if not specified.
	BufferSize int

	// EventTypes specifies which event types to receive.
	// Empty means all event types.
	EventTypes []EventType

	// BufferOverflowPolicy defines behavior when the event buffer is full.
	// Defaults to OverflowPolicyDropNewest.
	BufferOverflowPolicy BufferOverflowPolicy
}

// DefaultWatchOptions returns a WatchOptions with sensible defaults.
func DefaultWatchOptions() WatchOptions {
	return WatchOptions{
		BufferSize: DefaultWatchBufferSize,
	}
}

// WatchResult contains the event channel and control functions for a watch operation.
type WatchResult struct {
	// Events is the channel that delivers policy events.
	Events <-chan PolicyEvent

	// Stop cancels the watch operation and closes the events channel.
	Stop func()

	// Done returns a channel that's closed when the watch operation stops.
	Done <-chan struct{}
}
