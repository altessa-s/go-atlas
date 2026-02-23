// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"context"
	"time"
)

const (
	// Default buffer size for watch events
	defaultWatchBufferSize = 100
)

// EventType represents the type of secret change event that occurred during
// watch operations. It indicates whether a secret was created, updated, deleted,
// or if an error occurred during the watch operation.
type EventType int

const (
	// EventTypeCreated indicates a new secret was created
	EventTypeCreated EventType = iota
	// EventTypeUpdated indicates an existing secret was modified
	EventTypeUpdated
	// EventTypeDeleted indicates a secret was removed
	EventTypeDeleted
	// EventTypeError indicates an error occurred during watching
	EventTypeError
)

// BufferOverflowPolicy defines behavior when the event channel buffer is full.
type BufferOverflowPolicy int

const (
	// OverflowPolicyDropNewest drops the new event if buffer is full (default).
	// This is the safest option as it doesn't block the producer.
	OverflowPolicyDropNewest BufferOverflowPolicy = iota

	// OverflowPolicyDropOldest drops the oldest event from buffer to make room for new one.
	// Use when latest events are more important than older ones.
	OverflowPolicyDropOldest

	// OverflowPolicyBlock blocks until space is available or context is canceled.
	// Use when no events should be lost, but be aware this can slow down the producer.
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

// String returns a string representation of the event type
func (e EventType) String() string {
	switch e {
	case EventTypeCreated:
		return "created"
	case EventTypeUpdated:
		return "updated"
	case EventTypeDeleted:
		return "deleted"
	case EventTypeError:
		return "error"
	default:
		return "unknown"
	}
}

// WatchEvent represents a change event for a secret, containing all information
// about what changed, when it changed, and the before/after values. Events are
// generated during Manager cache updates and distributed to all active watchers
// that match the event's filtering criteria.
type WatchEvent[T any] struct {
	// Type is the type of change that occurred
	Type EventType

	// Key is the secret key that changed
	Key string

	// Value is the new secret value (nil for delete events)
	Value *Value[T]

	// PreviousValue is the previous secret value (nil for create events)
	PreviousValue *Value[T]

	// OccurredTime is when the event occurred
	OccurredTime time.Time

	// Error contains error details for error events
	Error error

	// Source identifies which provider generated the event
	Source string
}

// WatchOptions configures the behavior of watch operations, including which
// secrets to monitor, what types of events to receive, and operational parameters
// like buffer sizes. All fields are optional and have sensible defaults.
type WatchOptions struct {
	// Keys specifies which keys to watch (empty = watch all)
	Keys []string

	// EventTypes specifies which event types to receive (empty = all types)
	EventTypes []EventType

	// BufferSize is the size of the event channel buffer (default: 100)
	BufferSize int

	// BufferOverflowPolicy defines behavior when buffer is full (default: DropNewest).
	// See OverflowPolicyDropNewest, OverflowPolicyDropOldest, OverflowPolicyBlock.
	BufferOverflowPolicy BufferOverflowPolicy

	// SkipInitialEvents when true, suppresses the initial batch of EventTypeCreated
	// events that would normally be sent for all existing secrets when a watch starts.
	// Only events for changes occurring after the watch is established will be delivered.
	// Default is false (initial events are sent).
	SkipInitialEvents bool
}

// DefaultWatchOptions returns a WatchOptions struct with sensible default values
// for all configuration fields. This is the recommended starting point for most
// watch operations, with specific options modified as needed.
func DefaultWatchOptions() WatchOptions {
	return WatchOptions{
		BufferSize: defaultWatchBufferSize,
	}
}

// WatchResult contains the event channel and control functions for a watch operation.
// It provides access to the event stream and methods to control the watch lifecycle.
// The Events channel will be closed when the watch operation stops.
type WatchResult[T any] struct {
	// Events is the channel that delivers watch events
	Events <-chan WatchEvent[T]

	// Stop cancels the watch operation and closes the events channel
	Stop func()

	// Done returns a channel that's closed when the watch operation stops
	Done <-chan struct{}
}

// Watcher defines the interface for watching secret changes. Implementations
// must provide real-time notifications when secrets are created, updated, or deleted.
// The interface supports flexible configuration through WatchOptions and provides
// lifecycle management through the returned WatchResult.
type Watcher[T any] interface {
	// Watch starts watching for secret changes and returns a channel of events.
	// The watch operation continues until the context is canceled or Stop() is called.
	//
	// Parameters:
	//   - ctx: context for the watch operation, supports cancellation
	//   - opts: configuration options for the watch behavior
	//
	// Returns a WatchResult containing the event channel and control functions.
	Watch(ctx context.Context, opts WatchOptions) (*WatchResult[T], error)
}

// WatchFilter is a function type that determines whether a specific event should
// be delivered to a watcher. It receives a WatchEvent and returns true if the event
// should be delivered, or false if it should be filtered out. Multiple filters
// can be combined, and all must return true for an event to be delivered.
type WatchFilter[T any] func(event WatchEvent[T]) bool

// Common watch filters

// FilterByKeys creates a WatchFilter that only passes events for the specified keys.
// If no keys are provided, the filter allows all events (equivalent to no filtering).
// This is useful for monitoring only critical secrets or specific configuration groups.
func FilterByKeys[T any](keys ...string) WatchFilter[T] {
	if len(keys) == 0 {
		return nil // no filtering
	}

	keySet := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		keySet[key] = struct{}{}
	}

	return func(event WatchEvent[T]) bool {
		_, exists := keySet[event.Key]
		return exists
	}
}

// FilterByEventTypes creates a WatchFilter that only passes events of the specified types.
// If no types are provided, the filter allows all event types (equivalent to no filtering).
// This is useful for monitoring only specific types of changes, such as only deletions
// or only creations and updates.
func FilterByEventTypes[T any](types ...EventType) WatchFilter[T] {
	if len(types) == 0 {
		return nil // no filtering
	}

	typeSet := make(map[EventType]struct{}, len(types))
	for _, t := range types {
		typeSet[t] = struct{}{}
	}

	return func(event WatchEvent[T]) bool {
		_, exists := typeSet[event.Type]
		return exists
	}
}

// FilterBySource creates a WatchFilter that only passes events from the specified sources.
// If no sources are provided, the filter allows all sources (equivalent to no filtering).
// This is useful in multi-provider environments where you want to monitor changes
// from specific storage backends only.
func FilterBySource[T any](sources ...string) WatchFilter[T] {
	if len(sources) == 0 {
		return nil // no filtering
	}

	sourceSet := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		sourceSet[source] = struct{}{}
	}

	return func(event WatchEvent[T]) bool {
		_, exists := sourceSet[event.Source]
		return exists
	}
}
