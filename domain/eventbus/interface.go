// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package eventbus

import "context"

// Handler handles an event delivered through the bus. It runs synchronously
// within the caller's context (and transaction). Returning an error stops
// further handlers and propagates out of Publish.
type Handler func(ctx context.Context, event any) error

// TypedHandler handles an event of a known static type E. It is the typed
// counterpart of Handler, registered through the generic Subscribe and
// RegisterAdapter helpers and invoked without a manual type assertion.
type TypedHandler[E any] func(ctx context.Context, event E) error

// Publisher publishes events to a bus. It is the minimal surface a producer
// depends on and the argument accepted by the generic Publish helper.
type Publisher interface {
	// Publish runs every handler and adapter registered for the event's type
	// synchronously, in registration order, stopping at the first error. It
	// returns only an error: the bus carries commands and notifications, not
	// request/response — never hand a value back by mutating event (see the
	// package doc's Boundaries section).
	Publish(ctx context.Context, event any) error
}

// Bus is the in-process event bus contract (for dependency injection and
// testing). Handlers carry business logic and run first; adapters run after all
// handlers for a type and are the integration point for post-handler side
// effects.
type Bus interface {
	Publisher

	// Subscribe registers a handler for a specific event type. The eventType
	// argument is a zero-value instance of the event type. Multiple handlers may
	// be registered for the same event type; they run in registration order.
	Subscribe(eventType any, handler Handler)

	// RegisterAdapter registers an adapter for a specific event type. Adapters
	// run after all handlers for that type, in registration order. An adapter
	// error stops the chain and propagates out of Publish, exactly like a handler.
	RegisterAdapter(eventType any, adapter Handler)

	// RegisterGlobalAdapter registers an adapter that runs for every event type,
	// after all type-specific handlers and adapters.
	RegisterGlobalAdapter(adapter Handler)

	// RequireHandler declares that the given event type must have at least one
	// handler. eventType is a zero-value instance of the event type. The
	// requirement is checked by Validate.
	RequireHandler(eventType any)

	// Validate returns an error if any type declared via RequireHandler has no
	// handler. It is meant to run once after wiring (e.g. an fx OnStart hook) so
	// a missing subscription fails startup instead of silently dropping events.
	Validate() error

	// InTransaction reports whether ctx carries an active transaction, per the
	// probe configured at construction. It is the basis of PublishTx and is
	// false when no probe is configured.
	InTransaction(ctx context.Context) bool
}
