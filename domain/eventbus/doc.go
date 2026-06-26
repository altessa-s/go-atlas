// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package eventbus provides a synchronous, lock-free, in-process event bus used
// to decouple domains that must coordinate without taking a direct dependency on
// one another.
//
// # Delivery and transaction semantics
//
// Handlers run synchronously in the caller's goroutine and context (and thus
// within the same database transaction when Publish is called from inside one).
// For a given event the bus runs, in registration order, the type-specific
// handlers, then the type-specific adapters, then the global adapters. If any of
// them returns an error, Publish stops and returns it — so a subscriber can veto
// the operation that produced the event (e.g. abort the enclosing transaction).
//
// Dispatch deliberately does not recover panics: a panicking handler must
// propagate to the caller so an enclosing transaction rolls back rather than
// committing partial work.
//
// # Locking
//
// Because dispatch is synchronous in the caller's goroutine, any application
// lock held when Publish is called stays held for the entire handler chain. Do
// not call Publish while holding a lock that a handler (directly, or through a
// call back into the publishing service) might try to acquire: that deadlocks
// the goroutine. Pass the state handlers need inside the event itself rather
// than letting them reach back into the publisher's internals. Re-entrant
// publishing of an in-flight event type is separately rejected with
// [ErrEventCycle] (and unbounded nesting with [ErrDispatchTooDeep]), but those
// guards do not detect a deadlock on a non-event lock — the invariant above is
// the caller's responsibility.
//
// # Typed API
//
// The generic helpers [Subscribe], [On], [RegisterAdapter] and [Publish] are a
// type-safe facade over the untyped [Bus]. They derive the event key from the
// static type parameter, so a typed publish always reaches its typed subscribers
// and handlers no longer need a manual type assertion. The type parameter must
// be a concrete (non-interface) type.
//
// # Adapters
//
// Adapters are the extension point for post-handler side effects: they run after
// all business handlers and share the caller's context and transaction, and an
// adapter error aborts the publish exactly like a handler error.
//
// # Boundaries
//
// The bus is for commands, cascades, vetoes and notifications — interactions
// whose only reply is success or failure. It is not a request/response channel:
// Publish returns only an error, so a subscriber cannot hand a value back to the
// publisher. Do not smuggle a result out by mutating the event: with several
// handlers that races, and it hides the real contract. When a caller needs a
// value from another domain, depend on a narrow interface of that domain
// instead — the notification then flows one way through an event and the query
// the other way through the interface, so the dependency graph stays acyclic.
//
// # Observability
//
// The core bus emits no metrics. Wrap it with [NewObserved] to record publish
// latency and outcome per event type through a metrics collector.
package eventbus
