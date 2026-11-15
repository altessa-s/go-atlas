// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package eventbus

import "context"

// Subscribe registers a typed handler for events of type E on the bus. It is the
// type-safe counterpart of Bus.Subscribe: the handler receives the event already
// asserted to E, so no manual type assertion is needed. E must be a concrete
// (non-interface) type; a typed Publish[E] reaches every handler registered with
// the same E.
func Subscribe[E any](b Bus, handler TypedHandler[E]) {
	b.Subscribe(zeroEvent[E](), wrapTyped(handler))
}

// RegisterAdapter registers a typed adapter for events of type E. Like
// Bus.RegisterAdapter, adapters run after all handlers for the type.
func RegisterAdapter[E any](b Bus, adapter TypedHandler[E]) {
	b.RegisterAdapter(zeroEvent[E](), wrapTyped(adapter))
}

// Publish publishes a typed event through the publisher. The event is keyed by
// its own type E, matching subscriptions registered with Subscribe[E].
func Publish[E any](ctx context.Context, p Publisher, event E) error {
	return p.Publish(ctx, event)
}

// Require declares that events of type E must have at least one handler,
// checked by Bus.Validate. It is the typed counterpart of Bus.RequireHandler and
// keys on the same type E as Subscribe[E]. E must be a concrete type.
func Require[E any](b Bus) {
	b.RequireHandler(zeroEvent[E]())
}

// zeroEvent returns a zero value of E boxed in an any, used purely so the
// untyped registration keys on reflect.TypeOf(E). For a pointer type E the zero
// value is a typed nil pointer, which still carries the *E type in the interface.
func zeroEvent[E any]() any {
	var zero E
	return zero
}

// wrapTyped adapts a TypedHandler[E] to an untyped Handler, performing the E
// assertion once. An event of another dynamic type is ignored (the bus only
// dispatches matching types, so this guards against manual misuse).
func wrapTyped[E any](handler TypedHandler[E]) Handler {
	return func(ctx context.Context, event any) error {
		typed, ok := event.(E)
		if !ok {
			return nil
		}
		return handler(ctx, typed)
	}
}
